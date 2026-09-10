-- Store structured evolution edges (from -> to species, level threshold, trigger)
-- alongside the flat member list. Used by the evolution feature to decide when a
-- Pokemon can evolve after leveling up.
ALTER TABLE evolution_chain
    ADD COLUMN IF NOT EXISTS links JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Link player cards to their canonical pokemon row so evolution can resolve the
-- card's evolution chain. Nullable: legacy rows and cards for Pokemon not yet
-- cached in the pokemon table will have pokemon_id = NULL and fall back to a
-- lazy name-based lookup at evolution-check time (see internal/battle/rewards.go).
ALTER TABLE player_cards
    ADD COLUMN IF NOT EXISTS pokemon_id INTEGER REFERENCES pokemon(id) ON DELETE SET NULL;

-- Backfill existing cards by pokemon name (pokemon.name is UNIQUE).
UPDATE player_cards pc
SET pokemon_id = p.id
FROM pokemon p
WHERE pc.pokemon_id IS NULL
  AND LOWER(p.name) = LOWER(pc.pokemon_name);

CREATE INDEX IF NOT EXISTS idx_player_cards_pokemon_id ON player_cards(pokemon_id);
