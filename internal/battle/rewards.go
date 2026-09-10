package battle

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"pokemon-cli/internal/database"
	"pokemon-cli/internal/middleware"
	"pokemon-cli/internal/pokemon"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ComprehensiveRewards struct {
	CoinsEarned               int                              `json:"coins_earned"`
	XPGains                   []PokemonXPGain                  `json:"xp_gains"`
	NewlyUnlockedAchievements []database.AchievementWithStatus `json:"newly_unlocked_achievements,omitempty"`
	BattleHistoryRecorded     bool                             `json:"battle_history_recorded"`
	StatsUpdated              bool                             `json:"stats_updated"`
}

type BattleRewards struct {
	CoinsEarned int           `json:"coins_earned"`
	XPGained    map[int]int   `json:"xp_gained"` // cardID -> XP amount
	LevelUps    []LevelUpInfo `json:"level_ups"` // Cards that leveled up
}

// LevelUpInfo contains information about a Pokemon that leveled up (legacy)
type LevelUpInfo struct {
	CardID     int    `json:"card_id"`
	Name       string `json:"name"`
	OldLevel   int    `json:"old_level"`
	NewLevel   int    `json:"new_level"`
	NewHP      int    `json:"new_hp"`
	NewAttack  int    `json:"new_attack"`
	NewDefense int    `json:"new_defense"`
	NewSpeed   int    `json:"new_speed"`
}

func CalculateAllRewards(bs *BattleState) *ComprehensiveRewards {
	rewards := &ComprehensiveRewards{
		XPGains:                   []PokemonXPGain{},
		NewlyUnlockedAchievements: []database.AchievementWithStatus{},
		BattleHistoryRecorded:     false,
		StatsUpdated:              false,
	}

	// Calculate coins based on mode and outcome
	switch bs.Winner {
	case "player":
		// Win rewards
		switch bs.Mode {
		case "1v1":
			rewards.CoinsEarned = 50
		case "5v5":
			rewards.CoinsEarned = 150
		}
	case "draw":
		// Draw rewards (more than loss, less than win)
		switch bs.Mode {
		case "1v1":
			rewards.CoinsEarned = 25
		case "5v5":
			rewards.CoinsEarned = 75
		}
	default:
		// Loss consolation coins
		rewards.CoinsEarned = 10
	}

	// Note: XP gains will be populated after applying XP and level ups
	// Note: Achievements will be populated after checking achievements

	return rewards
}

// CalculateRewards calculates coins and XP rewards based on battle outcome (legacy)
func CalculateRewards(bs *BattleState) *BattleRewards {
	rewards := &BattleRewards{
		XPGained: make(map[int]int),
		LevelUps: []LevelUpInfo{},
	}

	// Calculate coins based on mode and outcome
	if bs.Winner == "player" {
		switch bs.Mode {
		case "1v1":
			rewards.CoinsEarned = 50
		case "5v5":
			rewards.CoinsEarned = 150
		}
	} else {
		// Consolation coins for losing
		rewards.CoinsEarned = 10
	}

	// Calculate XP for participating Pokemon
	if bs.Winner == "player" {
		switch bs.Mode {
		case "1v1":
			// Award 20 XP to the winning Pokemon
			playerCard := bs.GetActivePlayerCard()
			if playerCard != nil {
				rewards.XPGained[playerCard.CardID] = 20
			}
		case "5v5":
			// Award 15 XP to each Pokemon that participated (had HP > 0 at some point)
			for _, card := range bs.PlayerDeck {
				// Award XP to all cards (they all participated in 5v5)
				rewards.XPGained[card.CardID] = 15
			}
		}
	}

	return rewards
}

// ApplyRewards applies rewards to the database (coins and XP)
func ApplyRewards(ctx context.Context, db *pgxpool.Pool, userID int, bs *BattleState, rewards *BattleRewards) error {
	// Start a transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Update user coins
	_, err = tx.Exec(ctx, `
		UPDATE users
		SET coins = coins + $1
		WHERE id = $2
	`, rewards.CoinsEarned, userID)
	if err != nil {
		return fmt.Errorf("failed to update coins: %w", err)
	}

	// Apply XP to each card and check for level ups
	for cardID, xp := range rewards.XPGained {
		// Get current card stats
		var currentLevel, currentXP, baseHP, baseAttack, baseDefense, baseSpeed int
		var pokemonName string
		err = tx.QueryRow(ctx, `
			SELECT level, xp, base_hp, base_attack, base_defense, base_speed, pokemon_name
			FROM player_cards
			WHERE id = $1 AND user_id = $2
		`, cardID, userID).Scan(&currentLevel, &currentXP, &baseHP, &baseAttack, &baseDefense, &baseSpeed, &pokemonName)
		if err != nil {
			return fmt.Errorf("failed to get card stats: %w", err)
		}

		// Add XP
		newXP := currentXP + xp
		newLevel := currentLevel

		// Check for level ups (max level 50)
		for newLevel < 50 {
			xpRequired := 100 // flat cost per level — keeps evolution reachable
			if newXP >= xpRequired {
				newXP -= xpRequired
				newLevel++
			} else {
				break
			}
		}

		// If leveled up, calculate new stats
		if newLevel > currentLevel {
			// Calculate new stats based on level
			multiplier := 1.0 + (float64(newLevel-1) * 0.03) // 3% per level for HP
			newHP := int(float64(baseHP) * multiplier)
			newAttack := int(float64(baseAttack) * (1.0 + float64(newLevel-1)*0.02))
			newDefense := int(float64(baseDefense) * (1.0 + float64(newLevel-1)*0.02))
			newSpeed := int(float64(baseSpeed) * (1.0 + float64(newLevel-1)*0.01))

			// Update card in database
			_, err = tx.Exec(ctx, `
				UPDATE player_cards
				SET level = $1, xp = $2
				WHERE id = $3 AND user_id = $4
			`, newLevel, newXP, cardID, userID)
			if err != nil {
				return fmt.Errorf("failed to update card level: %w", err)
			}

			// Add to level up info
			rewards.LevelUps = append(rewards.LevelUps, LevelUpInfo{
				CardID:     cardID,
				Name:       pokemonName,
				OldLevel:   currentLevel,
				NewLevel:   newLevel,
				NewHP:      newHP,
				NewAttack:  newAttack,
				NewDefense: newDefense,
				NewSpeed:   newSpeed,
			})
		} else {
			// Just update XP
			_, err = tx.Exec(ctx, `
				UPDATE player_cards
				SET xp = $1
				WHERE id = $2 AND user_id = $3
			`, newXP, cardID, userID)
			if err != nil {
				return fmt.Errorf("failed to update card XP: %w", err)
			}
		}
	}

	// Commit transaction
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func ApplyAllRewards(ctx context.Context, db *pgxpool.Pool, userID int, bs *BattleState, rewards *ComprehensiveRewards, statsService StatsService, repo *Repository, pokemonService pokemon.PokemonService) error {
	// Resolve pokemon_id AND pre-fetch evolution targets for all cards that may
	// level up BEFORE opening the transaction. GetEvolutionForLevel can trigger
	// PokeAPI fetches (loadEvolutionChain cold refresh + GetByID for the target);
	// doing this inside db.Begin holds a pool connection during multi-second
	// network calls and risks pool exhaustion under concurrent load.
	xpMap := CalculateXPForBattle(bs)
	pokemonIDs := make(map[int]int)             // cardID -> pokemonID (0 = unresolvable)

	if pokemonService != nil {
		// First pass: resolve all pokemon_id values (may call PokeAPI for legacy cards)
		for cardID := range xpMap {
			var pokemonID *int
			err := db.QueryRow(ctx, `
				SELECT pokemon_id FROM player_cards WHERE id = $1 AND user_id = $2
			`, cardID, userID).Scan(&pokemonID)
			if err != nil {
				slog.Warn("evolution pre-fetch: could not read pokemon_id", "card_id", cardID, "error", err)
				continue
			}
			if pokemonID == nil {
				// Legacy card: resolve by name — may call PokeAPI, must be outside tx
				var pokemonName string
				if err := db.QueryRow(ctx, `SELECT pokemon_name FROM player_cards WHERE id = $1`, cardID).Scan(&pokemonName); err == nil {
					p, err := pokemonService.GetByName(ctx, pokemonName)
					if err == nil {
						// Backfill for future battles
						_, _ = db.Exec(ctx, `UPDATE player_cards SET pokemon_id = $1 WHERE id = $2`, p.ID, cardID)
						pokemonIDs[cardID] = p.ID
					}
				}
			} else {
				pokemonIDs[cardID] = *pokemonID
			}
		}

		// Second pass: pre-fetch evolution targets for cards that are likely to level up.
		// We optimistically check all participating cards — GetEvolutionForLevel returns
		// nil quickly if no evolution applies (cache hit), and only does network I/O
		// the first time a chain is encountered.
		for cardID, pid := range pokemonIDs {
			if pid == 0 {
				continue
			}
			// We don't know the new level yet, so fetch the current level to estimate.
			// Worst case: we fetch a target that ends up not needed (e.g. card doesn't
			// level up). That's a cheap no-op since the result is cached.
			var currentLevel int
			if err := db.QueryRow(ctx, `SELECT level FROM player_cards WHERE id = $1`, cardID).Scan(&currentLevel); err != nil {
				continue
			}
			// Check at current level + 1 as the minimum possible new level.
			// applyEvolution will re-check at the actual new level inside the tx,
			// but by then the chain and target are already in Redis/Postgres cache
			// so no PokeAPI call will happen.
			if _, err := pokemonService.GetEvolutionForLevel(ctx, pid, currentLevel+1); err != nil {
				slog.Warn("evolution pre-fetch failed", "card_id", cardID, "error", err)
			}
		}
	}

	// Start a transaction for consistency
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE users
		SET coins = coins + $1
		WHERE id = $2
	`, rewards.CoinsEarned, userID)
	if err != nil {
		return fmt.Errorf("failed to update coins: %w", err)
	}

	// Apply XP and handle level-ups within the transaction
	for cardID, xpGained := range xpMap {
		// Get current card data
		query := `
			SELECT id, user_id, pokemon_name, level, xp, base_hp, base_attack, base_defense, base_speed
			FROM player_cards
			WHERE id = $1 AND user_id = $2
		`

		var id, uid, level, xp, baseHP, baseAttack, baseDefense, baseSpeed int
		var pokemonName string

		err := tx.QueryRow(ctx, query, cardID, userID).Scan(
			&id, &uid, &pokemonName, &level, &xp,
			&baseHP, &baseAttack, &baseDefense, &baseSpeed,
		)
		if err != nil {
			return fmt.Errorf("failed to get card %d: %w", cardID, err)
		}

		// Calculate old stats
		oldStats := calculateStatsForLevel(level, baseHP, baseAttack, baseDefense, baseSpeed)

		// Track level ups
		oldLevel := level
		newXP := xp + xpGained
		newLevel := level

		// Process level ups
		for newLevel < 50 {
			xpRequired := 100 // flat cost per level — keeps evolution reachable
			if newXP >= xpRequired {
				newXP -= xpRequired
				newLevel++
			} else {
				break
			}
		}

		// Cap at level 50
		if newLevel >= 50 {
			newLevel = 50
			newXP = 0
		}

		// Evolution: use the pre-resolved pokemonID (resolved before db.Begin
		// to avoid holding a pool connection during PokeAPI fetches).
		var evolvedInto *pokemon.Pokemon
		if pokemonService != nil && newLevel > oldLevel {
			if pid, ok := pokemonIDs[cardID]; ok && pid != 0 {
				target, err := applyEvolution(ctx, tx, pokemonService, cardID, userID, pid, newLevel, pokemonName)
				if err != nil {
					slog.Warn("evolution check failed", "card_id", cardID, "error", err)
				} else {
					evolvedInto = target
				}
			}
		}

		// Calculate new stats (from the evolved base stats when evolution happened)
		statBaseHP, statBaseAttack, statBaseDefense, statBaseSpeed := baseHP, baseAttack, baseDefense, baseSpeed
		if evolvedInto != nil {
			statBaseHP, statBaseAttack, statBaseDefense, statBaseSpeed = evolvedBaseStats(evolvedInto)
		}
		newStats := calculateStatsForLevel(newLevel, statBaseHP, statBaseAttack, statBaseDefense, statBaseSpeed)

		// Update database
		updateQuery := `
			UPDATE player_cards
			SET level = $1, xp = $2, updated_at = $3
			WHERE id = $4
		`

		_, err = tx.Exec(ctx, updateQuery, newLevel, newXP, time.Now(), cardID)
		if err != nil {
			return fmt.Errorf("failed to update card %d: %w", cardID, err)
		}

		// Record XP gain
		xpGain := PokemonXPGain{
			CardID:      cardID,
			PokemonName: pokemonName,
			XPGained:    xpGained,
			OldLevel:    oldLevel,
			NewLevel:    newLevel,
			LeveledUp:   newLevel > oldLevel,
		}

		// Include stat changes if leveled up
		if xpGain.LeveledUp {
			xpGain.OldHP = oldStats.HP
			xpGain.NewHP = newStats.HP
			xpGain.OldAttack = oldStats.Attack
			xpGain.NewAttack = newStats.Attack
			xpGain.OldDefense = oldStats.Defense
			xpGain.NewDefense = newStats.Defense
			xpGain.OldSpeed = oldStats.Speed
			xpGain.NewSpeed = newStats.Speed
		}

		if evolvedInto != nil {
			xpGain.Evolved = true
			xpGain.EvolvedFrom = pokemonName
			xpGain.EvolvedInto = evolvedInto.Name
			xpGain.NewSprite = evolvedInto.SpriteURL
		}

		rewards.XPGains = append(rewards.XPGains, xpGain)
	}

	duration := int(time.Since(bs.CreatedAt).Seconds())
	result := "loss"
	switch bs.Winner {
	case "player":
		result = "win"
	case "draw":
		result = "draw"
	}
	middleware.BattleResultTotal.WithLabelValues(result).Inc()

	_, err = tx.Exec(ctx, `
		INSERT INTO battle_history (user_id, mode, result, coins_earned, duration)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, bs.Mode, result, rewards.CoinsEarned, duration)
	if err != nil {
		return fmt.Errorf("failed to record battle history: %w", err)
	}
	rewards.BattleHistoryRecorded = true

	if repo != nil {
		err = repo.UpdatePlayerStatsInTx(ctx, tx, userID, bs.Mode, result, rewards.CoinsEarned)
		if err != nil {
			return fmt.Errorf("failed to update player stats: %w", err)
		}
		rewards.StatsUpdated = true
	}

	for _, gain := range rewards.XPGains {
		if gain.LeveledUp && gain.NewLevel > 1 {
			_, err = tx.Exec(ctx, `
				INSERT INTO player_stats (user_id, highest_level, updated_at)
				VALUES ($1, $2, NOW())
				ON CONFLICT (user_id) DO UPDATE
				SET highest_level = GREATEST(player_stats.highest_level, $2),
				    updated_at = NOW()
			`, userID, gain.NewLevel)
			if err != nil {
				return fmt.Errorf("failed to update highest level: %w", err)
			}
		}
	}

	// Commit transaction
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	if statsService != nil {
		newlyUnlocked, err := statsService.CheckAndUnlockAchievements(ctx, userID)
		if err == nil && len(newlyUnlocked) > 0 {
			rewards.NewlyUnlockedAchievements = newlyUnlocked
		}
		// Don't fail if achievement checking fails
	}

	return nil
}

// RecordBattleHistory records the battle outcome in the database (legacy)
func RecordBattleHistory(ctx context.Context, db *pgxpool.Pool, userID int, bs *BattleState, coinsEarned int) error {
	result := "loss"
	switch bs.Winner {
	case "player":
		result = "win"
	case "draw":
		result = "draw"
	}

	// Insert battle history
	_, err := db.Exec(ctx, `
		INSERT INTO battle_history (user_id, mode, result, coins_earned, duration)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, bs.Mode, result, coinsEarned, 0) // Duration can be calculated if needed
	if err != nil {
		return fmt.Errorf("failed to record battle history: %w", err)
	}

	// Update player stats
	var column string
	switch bs.Mode {
	case "1v1":
		if result == "win" {
			column = "wins_1v1"
		} else if result == "draw" {
			column = "draws_1v1"
		} else {
			column = "losses_1v1"
		}
	case "5v5":
		if result == "win" {
			column = "wins_5v5"
		} else if result == "draw" {
			column = "draws_5v5"
		} else {
			column = "losses_5v5"
		}
	}

	if column != "" {
		_, err = db.Exec(ctx, fmt.Sprintf(`
			INSERT INTO player_stats (user_id, %s, total_coins_earned)
			VALUES ($1, 1, $2)
			ON CONFLICT (user_id) DO UPDATE
			SET %s = player_stats.%s + 1,
			    total_coins_earned = player_stats.total_coins_earned + $2
		`, column, column, column), userID, coinsEarned)
		if err != nil {
			return fmt.Errorf("failed to update player stats: %w", err)
		}
	}

	return nil
}

// GetCurrentStats calculates current stats for a card based on level
func GetCurrentStats(baseHP, baseAttack, baseDefense, baseSpeed, level int) database.CardStats {
	multiplier := 1.0 + (float64(level-1) * 0.03) // 3% per level for HP
	return database.CardStats{
		HP:      int(float64(baseHP) * multiplier),
		Attack:  int(float64(baseAttack) * (1.0 + float64(level-1)*0.02)),
		Defense: int(float64(baseDefense) * (1.0 + float64(level-1)*0.02)),
		Speed:   int(float64(baseSpeed) * (1.0 + float64(level-1)*0.01)),
		Stamina: int(float64(baseSpeed) * 2 * (1.0 + float64(level-1)*0.01)),
	}
}

func AddAIPokemonToCollection(ctx context.Context, db *pgxpool.Pool, userID int, aiCard BattleCard) (*database.PlayerCard, error) {
	// Convert types and moves to JSON
	typesJSON, err := json.Marshal(aiCard.Types)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal types: %w", err)
	}

	movesJSON, err := json.Marshal(aiCard.Moves)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal moves: %w", err)
	}

	// Create the player card at level 1 with 0 XP
	playerCard := &database.PlayerCard{
		UserID:       userID,
		PokemonName:  aiCard.Name,
		Level:        1,
		XP:           0,
		BaseHP:       aiCard.HPMax,
		BaseAttack:   aiCard.Attack,
		BaseDefense:  aiCard.Defense,
		BaseSpeed:    aiCard.Speed,
		Types:        typesJSON,
		Moves:        movesJSON,
		Sprite:       aiCard.Sprite,
		IsLegendary:  false, // Will be determined by the Pokemon name
		IsMythical:   false, // Will be determined by the Pokemon name
		InDeck:       false, // Not added to deck automatically
		DeckPosition: nil,
	}

	// Check if legendary or mythical based on Pokemon name
	// This is a simple check - can be enhanced with a proper lookup table
	legendaryNames := map[string]bool{
		"articuno": true, "zapdos": true, "moltres": true, "mewtwo": true, "mew": true,
		"raikou": true, "entei": true, "suicune": true, "lugia": true, "ho-oh": true,
		"regirock": true, "regice": true, "registeel": true, "latias": true, "latios": true,
		"kyogre": true, "groudon": true, "rayquaza": true, "jirachi": true, "deoxys": true,
	}
	mythicalNames := map[string]bool{
		"mew": true, "celebi": true, "jirachi": true, "deoxys": true, "phione": true,
		"manaphy": true, "darkrai": true, "shaymin": true, "arceus": true, "victini": true,
		"keldeo": true, "meloetta": true, "genesect": true, "diancie": true, "hoopa": true,
		"volcanion": true, "magearna": true, "marshadow": true, "zeraora": true, "meltan": true,
		"melmetal": true, "zarude": true,
	}

	pokemonNameLower := strings.ToLower(aiCard.Name)
	playerCard.IsLegendary = legendaryNames[pokemonNameLower]
	playerCard.IsMythical = mythicalNames[pokemonNameLower]

	// Insert into database
	query := `
		INSERT INTO player_cards (
			user_id, pokemon_name, level, xp, base_hp, base_attack, base_defense, base_speed,
			types, moves, sprite, is_legendary, is_mythical, in_deck, deck_position
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, created_at, updated_at
	`

	err = db.QueryRow(ctx, query,
		playerCard.UserID, playerCard.PokemonName, playerCard.Level, playerCard.XP,
		playerCard.BaseHP, playerCard.BaseAttack, playerCard.BaseDefense, playerCard.BaseSpeed,
		playerCard.Types, playerCard.Moves, playerCard.Sprite,
		playerCard.IsLegendary, playerCard.IsMythical, playerCard.InDeck, playerCard.DeckPosition,
	).Scan(&playerCard.ID, &playerCard.CreatedAt, &playerCard.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to add Pokemon to collection: %w", err)
	}

	return playerCard, nil
}
