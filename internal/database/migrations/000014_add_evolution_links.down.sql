DROP INDEX IF EXISTS idx_player_cards_pokemon_id;
ALTER TABLE player_cards DROP COLUMN IF EXISTS pokemon_id;
ALTER TABLE evolution_chain DROP COLUMN IF EXISTS links;
