package battle

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"pokemon-cli/game/models"
	"pokemon-cli/internal/database"
	"pokemon-cli/internal/middleware"
	"pokemon-cli/internal/pokemon"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler handles battle-related HTTP requests
type Handler struct {
	sessions       map[string]*Session    // Legacy in-memory sessions for backward compatibility
	repo           *Repository            // Database repository for persistent storage
	statsService   StatsService           // Stats service for achievement checking
	tokenService   TokenService           // Token service for token management
	enemySelector  EnemySelector          // Anti-repeat enemy selection service
	pokemonService pokemon.PokemonService // Tiered pokemon fetch service
	mu             sync.RWMutex           // Mutex for thread-safe access to legacy sessions
}

// StatsService defines the interface for stats operations
type StatsService interface {
	CheckAndUnlockAchievements(ctx context.Context, userID int) ([]database.AchievementWithStatus, error)
}

// TokenService defines the interface for token operations
type TokenService interface {
	GetTokenBalance(ctx context.Context, userID int) (int, error)
	ConsumeToken(ctx context.Context, userID int, mode string) error
	CheckAndResetTokens(ctx context.Context, userID int) error
	GetTokenCostForMode(mode string) int
}

// Session holds core game state plus web-only turn state (legacy support)
type Session struct {
	State *models.GameState
	Turn  *TurnState
}

// NewHandler creates a new battle handler
func NewHandler(db *pgxpool.Pool, statsService StatsService, tokenService TokenService) *Handler {
	return &Handler{
		sessions:     make(map[string]*Session),
		repo:         NewRepository(db),
		statsService: statsService,
		tokenService: tokenService,
	}
}

// SetEnemySelector configures the EnemySelector for the Handler
func (h *Handler) SetEnemySelector(selector EnemySelector) {
	h.enemySelector = selector
}

// SetPokemonService configures the PokemonService for the Handler
func (h *Handler) SetPokemonService(ps pokemon.PokemonService) {
	h.pokemonService = ps
}

// ConvertPlayerCardToPokemonCard converts a database PlayerCard to a pokemon.Card for battles
func ConvertPlayerCardToPokemonCard(dbCard database.PlayerCard) pokemon.Card {
	// Parse types from JSON
	var types []string
	if err := json.Unmarshal(dbCard.Types, &types); err != nil {
		types = []string{"normal"} // Default fallback
	}

	// Parse moves from JSON
	var moves []pokemon.Move
	if err := json.Unmarshal(dbCard.Moves, &moves); err != nil {
		// Default fallback move
		moves = []pokemon.Move{
			{Name: "tackle", Power: 40, StaminaCost: 10, Type: "normal"},
		}
	}

	// Calculate current stats based on level
	stats := dbCard.GetCurrentStats()

	return pokemon.Card{
		CardID:      dbCard.ID, // Store the database card ID
		Name:        dbCard.PokemonName,
		HP:          stats.HP,
		HPMax:       stats.HP,
		Stamina:     stats.Stamina,
		Defense:     stats.Defense,
		Attack:      stats.Attack,
		Speed:       stats.Speed,
		Moves:       moves,
		Types:       types,
		Sprite:      dbCard.Sprite,
		Level:       dbCard.Level,
		XP:          dbCard.XP,
		IsLegendary: dbCard.IsLegendary,
		IsMythical:  dbCard.IsMythical,
	}
}

// GetBattleState retrieves a battle state by ID from database
func (h *Handler) GetBattleState(c *fiber.Ctx, id string) (*BattleState, error) {
	state, err := h.repo.GetBattleSession(c.Context(), id)
	if err != nil {
		return nil, err
	}
	return state, nil
}

// SaveBattleState stores a battle state to database
func (h *Handler) SaveBattleState(c *fiber.Ctx, state *BattleState) error {
	state.UpdatedAt = time.Now()
	return h.repo.SaveBattleSession(c.Context(), state)
}

// DeleteBattleState removes a battle state from database
func (h *Handler) DeleteBattleState(c *fiber.Ctx, id string) error {
	return h.repo.DeleteBattleSession(c.Context(), id)
}

// StartBattleEnhanced handles POST /api/battle/start with mode selection
func (h *Handler) StartBattleEnhanced(c *fiber.Ctx) error {
	// Get user ID from context (set by auth middleware)
	userID, ok := c.Locals("user_id").(int)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "UNAUTHORIZED",
				"message": "User not authenticated",
			},
		})
	}

	var req struct {
		Mode string `json:"mode"` // "1v1" or "5v5"
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "INVALID_REQUEST",
				"message": "Invalid request body",
			},
		})
	}

	// Sanitize and validate mode
	req.Mode = strings.TrimSpace(req.Mode)
	if req.Mode != "1v1" && req.Mode != "5v5" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "INVALID_MODE",
				"message": "Invalid battle mode. Must be '1v1' or '5v5'",
			},
		})
	}

	// Check and reset tokens if needed (24-hour reset)
	if err := h.tokenService.CheckAndResetTokens(c.Context(), userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "TOKEN_SYSTEM_ERROR",
				"message": "Failed to check token status",
			},
		})
	}

	// Get token cost for the selected mode
	tokenCost := h.tokenService.GetTokenCostForMode(req.Mode)

	// Verify user has enough tokens for this battle mode
	tokenBalance, err := h.tokenService.GetTokenBalance(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "TOKEN_SYSTEM_ERROR",
				"message": "Failed to retrieve token balance",
			},
		})
	}

	if tokenBalance < tokenCost {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "INSUFFICIENT_TOKENS",
				"message": fmt.Sprintf("You don't have enough tokens to start a %s battle. Required: %d, Available: %d. Purchase more tokens from the shop or wait for daily reset.", req.Mode, tokenCost, tokenBalance),
			},
		})
	}

	// Fetch player's deck from database
	playerDeckCards, err := h.repo.GetUserDeck(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "DATABASE_ERROR",
				"message": "Failed to fetch player deck",
			},
		})
	}

	// Check if player has a configured deck
	if len(playerDeckCards) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "NO_DECK_CONFIGURED",
				"message": "You must configure a deck before starting a battle. Please add Pokemon to your deck.",
			},
		})
	}

	// Validate deck size based on mode
	requiredCards := 1
	if req.Mode == "5v5" {
		requiredCards = 5
	}

	if len(playerDeckCards) < requiredCards {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "INSUFFICIENT_DECK_SIZE",
				"message": fmt.Sprintf("Your deck must have at least %d Pokemon for %s mode. Current deck size: %d", requiredCards, req.Mode, len(playerDeckCards)),
			},
		})
	}

	// Convert player's database cards to pokemon.Card format
	var playerDeck []pokemon.Card
	if req.Mode == "1v1" {
		// For 1v1, select a random Pokemon from the deck
		if len(playerDeckCards) > 0 {
			randomIdx := rand.Intn(len(playerDeckCards))
			playerDeck = append(playerDeck, ConvertPlayerCardToPokemonCard(playerDeckCards[randomIdx]))
		}
	} else {
		// For 5v5, use all cards in deck order
		for i := 0; i < requiredCards && i < len(playerDeckCards); i++ {
			playerDeck = append(playerDeck, ConvertPlayerCardToPokemonCard(playerDeckCards[i]))
		}
	}

	// Generate AI deck using anti-repeat EnemySelector
	var aiDeck []pokemon.Card
	aiCount := 1
	if req.Mode != "1v1" {
		aiCount = 5
	}
	for i := 0; i < aiCount; i++ {
		aiDeck = append(aiDeck, h.generateAICard(c.Context(), userID))
	}

	// Start the battle
	battleState, err := StartBattle(userID, req.Mode, playerDeck, aiDeck)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Save battle state to database BEFORE consuming tokens
	// This ensures we only consume tokens if the battle is successfully persisted
	if err := h.SaveBattleState(c, battleState); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "DATABASE_ERROR",
				"message": "Failed to save battle session",
			},
		})
	}

	// Consume tokens AFTER battle is successfully saved
	// This ensures tokens are only consumed if the battle actually starts and is persisted
	if err := h.tokenService.ConsumeToken(c.Context(), userID, req.Mode); err != nil {
		// Token consumption failed after battle was saved
		// Delete the battle state to maintain consistency
		_ = h.DeleteBattleState(c, battleState.ID)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "TOKEN_CONSUMPTION_FAILED",
				"message": "Failed to consume token. Please try again.",
			},
		})
	}

	// Return battle state with card visibility
	middleware.BattleStartTotal.WithLabelValues(req.Mode).Inc()
	response := BuildBattleResponse(battleState, []string{fmt.Sprintf("Battle started! Mode: %s", req.Mode)}, true)

	return c.JSON(response)
}

// StartBattle handles POST /api/battle/start (legacy support)
func (h *Handler) StartBattle(c *fiber.Ctx) error {
	var req struct {
		PlayerName string `json:"playerName"`
		AIName     string `json:"aiName"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Use random cards for both player and AI
	playerCard := pokemon.FetchRandomPokemonCard(false)
	aiCard := pokemon.FetchRandomPokemonCard(false)
	player := models.NewPlayer(req.PlayerName, []pokemon.Card{playerCard})
	ai := models.NewPlayer(req.AIName, []pokemon.Card{aiCard})

	state := &models.GameState{
		Player:          player,
		AI:              ai,
		BattleMode:      "1v1",
		PlayerActiveIdx: 0,
		AIActiveIdx:     0,
		BattleStarted:   true,
		InBattle:        true,
		TurnNumber:      1,
	}

	turn := &TurnState{WhoseTurn: "player"}
	id := uuid.New().String()

	h.sessions[id] = &Session{State: state, Turn: turn}

	middleware.BattleStartTotal.WithLabelValues(state.BattleMode).Inc()
	return c.JSON(fiber.Map{
		"session": id,
		"state":   state,
		"turn":    turn,
	})
}

// MakeMove handles POST /api/battle/move
func (h *Handler) MakeMove(c *fiber.Ctx) error {
	var req struct {
		Session string `json:"session"`
		Move    string `json:"move"`
		MoveIdx *int   `json:"moveIdx"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	sess, ok := h.sessions[req.Session]
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Session not found"})
	}

	// Call ProcessWebMove to process the move and update state
	result, err := ProcessWebMove(sess.State, sess.Turn, req.Move, req.MoveIdx)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

// MakeMoveEnhanced handles POST /api/battle/move with enhanced battle system
func (h *Handler) MakeMoveEnhanced(c *fiber.Ctx) error {
	// Get user ID from context
	userID, ok := c.Locals("user_id").(int)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "UNAUTHORIZED",
				"message": "User not authenticated",
			},
		})
	}

	var req struct {
		BattleID string `json:"battle_id"`
		Move     string `json:"move"`
		MoveIdx  *int   `json:"move_idx"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "INVALID_REQUEST",
				"message": "Invalid request body",
			},
		})
	}

	// Sanitize and validate inputs
	req.BattleID = strings.TrimSpace(req.BattleID)
	req.Move = strings.TrimSpace(req.Move)

	if req.BattleID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "INVALID_REQUEST",
				"message": "Battle ID is required",
			},
		})
	}

	// Validate move
	validMoves := map[string]bool{
		"attack": true, "defend": true, "pass": true, "sacrifice": true, "surrender": true,
	}
	if !validMoves[req.Move] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "INVALID_MOVE",
				"message": "Invalid move. Must be one of: attack, defend, pass, sacrifice, surrender",
			},
		})
	}

	// Validate move_idx for attack moves
	if req.Move == "attack" && (req.MoveIdx == nil || *req.MoveIdx < 0) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "INVALID_MOVE",
				"message": "Move index is required for attack moves",
			},
		})
	}

	// Get battle state from database
	battleState, err := h.GetBattleState(c, req.BattleID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "BATTLE_NOT_FOUND",
				"message": "Battle not found",
			},
		})
	}

	// Verify user owns this battle
	if battleState.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "FORBIDDEN",
				"message": "Not your battle",
			},
		})
	}

	// Process the move
	logEntries, err := ProcessMove(battleState, req.Move, req.MoveIdx)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "INVALID_MOVE",
				"message": err.Error(),
			},
		})
	}

	// Save updated battle state to database
	if err := h.SaveBattleState(c, battleState); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "DATABASE_ERROR",
				"message": "Failed to save battle state",
			},
		})
	}

	hideAICards := !battleState.BattleOver || battleState.Winner != "player" || battleState.Mode != "5v5"

	response := BuildBattleResponse(battleState, logEntries, hideAICards)

	if battleState.BattleOver {
		db, ok := c.Locals("db").(*pgxpool.Pool)
		if ok {
			// Calculate all rewards
			rewards := CalculateAllRewards(battleState)

			// Apply all rewards in a single transaction
			err := ApplyAllRewards(c.Context(), db, userID, battleState, rewards, h.statsService, h.repo, h.pokemonService)
			if err != nil {
				// Log error but don't fail the request - battle is already over
				fmt.Printf("Failed to apply rewards: %v\n", err)
				// Still return partial rewards info if available
				response["coins_earned"] = rewards.CoinsEarned
			} else {
				// Add comprehensive rewards to response
				response["coins_earned"] = rewards.CoinsEarned
				response["xp_gains"] = rewards.XPGains
				if len(rewards.NewlyUnlockedAchievements) > 0 {
					response["newly_unlocked_achievements"] = rewards.NewlyUnlockedAchievements
				}
				response["battle_history_recorded"] = rewards.BattleHistoryRecorded
				response["stats_updated"] = rewards.StatsUpdated
			}
		}
	}

	return c.JSON(response)
}

// GetBattleStateEnhanced handles GET /api/battle/state with enhanced system
func (h *Handler) GetBattleStateEnhanced(c *fiber.Ctx) error {
	// Get user ID from context
	userID, ok := c.Locals("user_id").(int)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	battleID := c.Query("battle_id")
	if battleID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "battle_id required"})
	}

	// Get battle state from database
	battleState, err := h.GetBattleState(c, battleID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Battle not found"})
	}

	// Verify user owns this battle
	if battleState.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Not your battle"})
	}

	hideAICards := !battleState.BattleOver || battleState.Winner != "player" || battleState.Mode != "5v5"

	response := BuildBattleResponse(battleState, []string{}, hideAICards)

	return c.JSON(response)
}

// SwitchPokemonHandler handles POST /api/battle/switch
func (h *Handler) SwitchPokemonHandler(c *fiber.Ctx) error {
	// Get user ID from context
	userID, ok := c.Locals("user_id").(int)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var req struct {
		BattleID string `json:"battle_id"`
		NewIdx   int    `json:"new_idx"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Get battle state from database
	battleState, err := h.GetBattleState(c, req.BattleID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Battle not found"})
	}

	// Verify user owns this battle
	if battleState.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Not your battle"})
	}

	// Switch Pokemon
	err = SwitchPokemon(battleState, req.NewIdx)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Save updated battle state to database
	if err := h.SaveBattleState(c, battleState); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save battle state"})
	}

	// Return updated state
	response := BuildBattleResponse(battleState, []string{fmt.Sprintf("Switched to %s", battleState.PlayerDeck[req.NewIdx].Name)}, true)

	return c.JSON(response)
}

func (h *Handler) GetBattleStateLegacy(c *fiber.Ctx) error {
	session := c.Query("session")
	sess, ok := h.sessions[session]
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Session not found"})
	}

	return c.JSON(fiber.Map{
		"state": sess.State,
		"turn":  sess.Turn,
	})
}

// CleanupExpiredSessions removes battle sessions older than 1 hour
func (h *Handler) CleanupExpiredSessions(c *fiber.Ctx) error {
	// Only allow admin or internal calls
	// For now, we'll make this a simple endpoint that can be called by a cron job

	count, err := h.repo.CleanupExpiredSessions(c.Context(), 1*time.Hour)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to cleanup expired sessions",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Cleanup completed",
		"deleted": count,
	})
}

func (h *Handler) SelectRewardHandler(c *fiber.Ctx) error {
	// Get user ID from context
	userID, ok := c.Locals("user_id").(int)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var req struct {
		BattleID     string `json:"battle_id"`
		PokemonIndex int    `json:"pokemon_index"` // Index of AI Pokemon to select (0-4)
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Get battle state from database
	battleState, err := h.GetBattleState(c, req.BattleID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Battle not found"})
	}

	// Verify user owns this battle
	if battleState.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Not your battle"})
	}

	// Validate battle was won by player and is 5v5 mode
	if battleState.Mode != "5v5" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Reward selection only available for 5v5 battles"})
	}

	if battleState.Winner != "player" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Can only select reward after winning the battle"})
	}

	if !battleState.BattleOver {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Battle is not over yet"})
	}

	// Check if reward already claimed
	if battleState.RewardClaimed {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Reward already claimed for this battle"})
	}

	// Validate Pokemon index
	if req.PokemonIndex < 0 || req.PokemonIndex >= len(battleState.AIDeck) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid Pokemon index"})
	}

	// Get the selected AI Pokemon
	selectedPokemon := battleState.AIDeck[req.PokemonIndex]

	// Get database connection from context
	db, ok := c.Locals("db").(*pgxpool.Pool)
	if !ok {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database connection not available"})
	}

	// Add the Pokemon to player's collection
	addedCard, err := AddAIPokemonToCollection(c.Context(), db, userID, selectedPokemon)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("Failed to add Pokemon to collection: %v", err)})
	}

	// Mark battle as reward claimed
	battleState.RewardClaimed = true
	if err := h.SaveBattleState(c, battleState); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save battle state"})
	}

	// Return success with the added card
	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("Successfully added %s to your collection!", selectedPokemon.Name),
		"card":    addedCard,
	})
}

// generateAICard produces an enemy Pokemon Card using EnemySelector anti-repeat logic with fallbacks
func (h *Handler) generateAICard(ctx context.Context, userID int) pokemon.Card {
	if h.enemySelector != nil {
		p, err := h.enemySelector.PickEnemy(ctx, SelectOptions{
			PlayerID:      userID,
			PokemonWindow: 5,
			FamilyWindow:  3,
		})
		if err == nil && p != nil {
			card := p.ToCard()
			if len(card.Moves) == 0 {
				card.Moves = []pokemon.Move{
					{Name: "tackle", Power: 40, StaminaCost: 13, Type: "normal"},
				}
			}
			return card
		}
	}

	if h.pokemonService != nil {
		card, err := h.pokemonService.GetRandomCard(ctx, false)
		if err == nil {
			return card
		}
	}

	// Ultimate fallback to legacy random generator
	return pokemon.FetchRandomPokemonCard(false)
}
