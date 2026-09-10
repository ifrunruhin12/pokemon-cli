package pokemon

import (
	"encoding/json"
	"time"
)

// BaseStats represents normalized Pokémon stats
type BaseStats struct {
	HP             int `json:"hp"`
	Attack         int `json:"attack"`
	Defense        int `json:"defense"`
	SpecialAttack  int `json:"special_attack"`
	SpecialDefense int `json:"special_defense"`
	Speed          int `json:"speed"`
}

// Pokemon represents a canonical Pokémon entity stored in PostgreSQL & Redis
type Pokemon struct {
	ID               int             `json:"id"`
	Name             string          `json:"name"`
	SpeciesID        int             `json:"species_id"`
	EvolutionChainID int             `json:"evolution_chain_id"`
	Generation       int             `json:"generation"`
	Types            []string        `json:"types"`
	BaseStats        BaseStats       `json:"base_stats"`
	Abilities        json.RawMessage `json:"abilities"`
	SpriteURL        string          `json:"sprite_url"`
	RawJSON          json.RawMessage `json:"raw_json"`
	FetchedAt        time.Time       `json:"fetched_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// EvolutionLink represents a single directed edge in an evolution chain
// (e.g. charmander -> charmeleon at level 16 via level-up).
type EvolutionLink struct {
	FromSpeciesID int    `json:"from"`
	ToSpeciesID   int    `json:"to"`
	MinLevel      int    `json:"min_level,omitempty"`
	Trigger       string `json:"trigger,omitempty"` // e.g. "level-up", "use-item", "trade"
}

// PickEvolutionLink returns the level-up evolution edge from speciesID whose
// minimum level has been reached at the given level. Non-level-up triggers
// (items, trades, etc.) are ignored — this game only evolves through leveling.
// Branching chains resolve deterministically to the lowest qualifying level.
// Returns nil when no evolution applies.
func PickEvolutionLink(links []EvolutionLink, speciesID int, level int) *EvolutionLink {
	var best *EvolutionLink
	for i := range links {
		l := &links[i]
		if l.FromSpeciesID != speciesID {
			continue
		}
		if l.Trigger != "level-up" || l.MinLevel <= 0 {
			continue
		}
		if level < l.MinLevel {
			continue
		}
		if best == nil || l.MinLevel < best.MinLevel {
			best = l
		}
	}
	return best
}

// EvolutionChain represents an evolution chain entity stored in PostgreSQL & Redis
type EvolutionChain struct {
	ID               int             `json:"id"`
	MemberSpeciesIDs []int           `json:"member_species_ids"`
	Links            []EvolutionLink `json:"links"`
	FetchedAt        time.Time       `json:"fetched_at"`
}

func cardHPFromBase(baseHP int) int {
	if baseHP <= 0 {
		baseHP = 50
	}
	return baseHP + baseHP/2
}

// CardBaseStats returns the four base stat values that get stored on player_cards,
// applying the same floor defaults and HP boost as ToCard(). Both ToCard() and
// evolvedBaseStats() in the battle package delegate here so the logic never
// diverges between the two code paths.
func (p *Pokemon) CardBaseStats() (hp, attack, defense, speed int) {
	hp = cardHPFromBase(p.BaseStats.HP)
	attack = p.BaseStats.Attack
	if attack <= 0 {
		attack = 40
	}
	defense = p.BaseStats.Defense
	if defense <= 0 {
		defense = 40
	}
	speed = p.BaseStats.Speed
	if speed <= 0 {
		speed = 50
	}
	return hp, attack, defense, speed
}

// ToCard converts a Pokemon domain entity into a battle Card
func (p *Pokemon) ToCard() Card {
	hp, attack, defense, speed := p.CardBaseStats()

	stamina := speed * 2
	isLegendary, isMythical := IsLegendaryOrMythical(p.Name)

	return Card{
		CardID:      p.ID,
		Name:        p.Name,
		HP:          hp,
		HPMax:       hp,
		Stamina:     stamina,
		Attack:      attack,
		Defense:     defense,
		Speed:       speed,
		Types:       p.Types,
		Sprite:      p.SpriteURL,
		Level:       1,
		XP:          0,
		IsLegendary: isLegendary,
		IsMythical:  isMythical,
	}
}
