package battle

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PokemonXPGain represents XP gained by a Pokemon after battle
type PokemonXPGain struct {
	CardID      int    `json:"card_id"`
	PokemonName string `json:"pokemon_name"`
	XPGained    int    `json:"xp_gained"`
	OldLevel    int    `json:"old_level"`
	NewLevel    int    `json:"new_level"`
	LeveledUp   bool   `json:"leveled_up"`
	OldHP       int    `json:"old_hp,omitempty"`
	NewHP       int    `json:"new_hp,omitempty"`
	OldAttack   int    `json:"old_attack,omitempty"`
	NewAttack   int    `json:"new_attack,omitempty"`
	OldDefense  int    `json:"old_defense,omitempty"`
	NewDefense  int    `json:"new_defense,omitempty"`
	OldSpeed    int    `json:"old_speed,omitempty"`
	NewSpeed    int    `json:"new_speed,omitempty"`

	// Evolution info (set when leveling up triggered an evolution)
	Evolved     bool   `json:"evolved"`
	EvolvedFrom string `json:"evolved_from,omitempty"`
	EvolvedInto string `json:"evolved_into,omitempty"`
	NewSprite   string `json:"new_sprite,omitempty"`
}

// CalculateXPForBattle calculates XP for all Pokemon that participated in battle
func CalculateXPForBattle(bs *BattleState) map[int]int {
	xpMap := make(map[int]int)

	// XP rewards are generous by design — evolution should be achievable through
	// normal play, not a multi-thousand-battle grind.
	// Win 1v1: 50 XP | Win 5v5: 40 XP per Pokemon
	// Draw 1v1: 25 XP | Draw 5v5: 20 XP
	// Loss: 15 XP (consolation — encourages continued play)
	var baseXP int

	switch bs.Winner {
	case "player":
		if bs.Mode == "1v1" {
			baseXP = 50
		} else {
			baseXP = 40
		}
	case "draw":
		if bs.Mode == "1v1" {
			baseXP = 25
		} else {
			baseXP = 20
		}
	default:
		baseXP = 15
	}

	switch bs.Mode {
	case "1v1":
		// In 1v1, find the Pokemon that actually participated
		// This is the Pokemon that took damage or was knocked out
		for _, card := range bs.PlayerDeck {
			if card.HP < card.HPMax || card.IsKnockedOut {
				xpMap[card.CardID] = baseXP
				break // Only one Pokemon participates in 1v1
			}
		}
	case "5v5":
		// In 5v5, all Pokemon that participated get XP
		// A Pokemon participated if it took damage or was knocked out
		for _, card := range bs.PlayerDeck {
			// Check if Pokemon participated (HP changed from max or was knocked out)
			if card.HP < card.HPMax || card.IsKnockedOut {
				xpMap[card.CardID] = baseXP
			}
		}
	}

	return xpMap
}

// ApplyXPAndLevelUps applies XP to Pokemon and handles level-ups
func ApplyXPAndLevelUps(ctx context.Context, db *pgxpool.Pool, userID int, xpMap map[int]int) ([]PokemonXPGain, error) {
	if len(xpMap) == 0 {
		return []PokemonXPGain{}, nil
	}

	results := []PokemonXPGain{}

	// Start a transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Process each Pokemon
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
			return nil, fmt.Errorf("failed to get card %d: %w", cardID, err)
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

		// Calculate new stats
		newStats := calculateStatsForLevel(newLevel, baseHP, baseAttack, baseDefense, baseSpeed)

		// Update database
		updateQuery := `
			UPDATE player_cards
			SET level = $1, xp = $2, updated_at = $3
			WHERE id = $4
		`

		_, err = tx.Exec(ctx, updateQuery, newLevel, newXP, time.Now(), cardID)
		if err != nil {
			return nil, fmt.Errorf("failed to update card %d: %w", cardID, err)
		}

		// Record result
		result := PokemonXPGain{
			CardID:      cardID,
			PokemonName: pokemonName,
			XPGained:    xpGained,
			OldLevel:    oldLevel,
			NewLevel:    newLevel,
			LeveledUp:   newLevel > oldLevel,
		}

		// Include stat changes if leveled up
		if result.LeveledUp {
			result.OldHP = oldStats.HP
			result.NewHP = newStats.HP
			result.OldAttack = oldStats.Attack
			result.NewAttack = newStats.Attack
			result.OldDefense = oldStats.Defense
			result.NewDefense = newStats.Defense
			result.OldSpeed = oldStats.Speed
			result.NewSpeed = newStats.Speed
		}

		results = append(results, result)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return results, nil
}

// Stats represents Pokemon stats at a given level
type Stats struct {
	HP      int
	Attack  int
	Defense int
	Speed   int
}

// calculateStatsForLevel calculates stats for a Pokemon at a given level
// Stat increases: HP +3%, Attack +2%, Defense +2%, Speed +1% per level
func calculateStatsForLevel(level int, baseHP, baseAttack, baseDefense, baseSpeed int) Stats {
	levelMultiplier := float64(level - 1)

	hp := int(float64(baseHP) * (1.0 + levelMultiplier*0.03))
	attack := int(float64(baseAttack) * (1.0 + levelMultiplier*0.02))
	defense := int(float64(baseDefense) * (1.0 + levelMultiplier*0.02))
	speed := int(float64(baseSpeed) * (1.0 + levelMultiplier*0.01))

	return Stats{
		HP:      hp,
		Attack:  attack,
		Defense: defense,
		Speed:   speed,
	}
}
