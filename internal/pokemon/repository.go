package pokemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrPokemonNotFound = errors.New("pokemon not found in repository")
var ErrEvolutionChainNotFound = errors.New("evolution chain not found in repository")

type PoolFilter struct {
	Generation *int
	MinLevel   int
	MaxLevel   int
}

type Repository interface {
	GetPokemon(ctx context.Context, id int) (*Pokemon, error)
	GetPokemonByName(ctx context.Context, name string) (*Pokemon, error)
	UpsertPokemon(ctx context.Context, p *Pokemon) error
	GetEvolutionChain(ctx context.Context, id int) (*EvolutionChain, error)
	UpsertEvolutionChain(ctx context.Context, ec *EvolutionChain) error
	CandidatePool(ctx context.Context, filter PoolFilter) ([]*Pokemon, error)
}

type postgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) GetPokemon(ctx context.Context, id int) (*Pokemon, error) {
	query := `
		SELECT id, name, species_id, evolution_chain_id, generation, types,
		       base_stats, abilities, COALESCE(sprite_url, ''), raw_json, fetched_at, updated_at
		FROM pokemon
		WHERE id = $1
	`
	row := r.db.QueryRow(ctx, query, id)

	var p Pokemon
	var baseStatsJSON, abilitiesJSON, rawJSON []byte

	err := row.Scan(
		&p.ID, &p.Name, &p.SpeciesID, &p.EvolutionChainID, &p.Generation,
		&p.Types, &baseStatsJSON, &abilitiesJSON, &p.SpriteURL, &rawJSON,
		&p.FetchedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPokemonNotFound
		}
		return nil, fmt.Errorf("error querying pokemon by id %d: %w", id, err)
	}

	if err := json.Unmarshal(baseStatsJSON, &p.BaseStats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal base_stats: %w", err)
	}
	p.Abilities = abilitiesJSON
	p.RawJSON = rawJSON

	return &p, nil
}

func (r *postgresRepository) GetPokemonByName(ctx context.Context, name string) (*Pokemon, error) {
	query := `
		SELECT id, name, species_id, evolution_chain_id, generation, types,
		       base_stats, abilities, COALESCE(sprite_url, ''), raw_json, fetched_at, updated_at
		FROM pokemon
		WHERE LOWER(name) = LOWER($1)
	`
	row := r.db.QueryRow(ctx, query, name)

	var p Pokemon
	var baseStatsJSON, abilitiesJSON, rawJSON []byte

	err := row.Scan(
		&p.ID, &p.Name, &p.SpeciesID, &p.EvolutionChainID, &p.Generation,
		&p.Types, &baseStatsJSON, &abilitiesJSON, &p.SpriteURL, &rawJSON,
		&p.FetchedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPokemonNotFound
		}
		return nil, fmt.Errorf("error querying pokemon by name %s: %w", name, err)
	}

	if err := json.Unmarshal(baseStatsJSON, &p.BaseStats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal base_stats: %w", err)
	}
	p.Abilities = abilitiesJSON
	p.RawJSON = rawJSON

	return &p, nil
}

func (r *postgresRepository) UpsertPokemon(ctx context.Context, p *Pokemon) error {
	baseStatsJSON, err := json.Marshal(p.BaseStats)
	if err != nil {
		return fmt.Errorf("failed to marshal base_stats: %w", err)
	}

	abilitiesJSON := p.Abilities
	if len(abilitiesJSON) == 0 {
		abilitiesJSON = []byte("[]")
	}

	rawJSON := p.RawJSON
	if len(rawJSON) == 0 {
		rawJSON = []byte("{}")
	}

	now := time.Now()
	if p.FetchedAt.IsZero() {
		p.FetchedAt = now
	}
	p.UpdatedAt = now

	query := `
		INSERT INTO pokemon (
			id, name, species_id, evolution_chain_id, generation, types,
			base_stats, abilities, sprite_url, raw_json, fetched_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12
		)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			species_id = EXCLUDED.species_id,
			evolution_chain_id = EXCLUDED.evolution_chain_id,
			generation = EXCLUDED.generation,
			types = EXCLUDED.types,
			base_stats = EXCLUDED.base_stats,
			abilities = EXCLUDED.abilities,
			sprite_url = EXCLUDED.sprite_url,
			raw_json = EXCLUDED.raw_json,
			updated_at = EXCLUDED.updated_at
	`

	_, err = r.db.Exec(ctx, query,
		p.ID, p.Name, p.SpeciesID, p.EvolutionChainID, p.Generation, p.Types,
		baseStatsJSON, abilitiesJSON, p.SpriteURL, rawJSON, p.FetchedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert pokemon %d (%s): %w", p.ID, p.Name, err)
	}
	return nil
}

func (r *postgresRepository) GetEvolutionChain(ctx context.Context, id int) (*EvolutionChain, error) {
	query := `
		SELECT id, member_species_ids, COALESCE(links, '[]'::jsonb), fetched_at
		FROM evolution_chain
		WHERE id = $1
	`
	row := r.db.QueryRow(ctx, query, id)

	var ec EvolutionChain
	var linksJSON []byte
	err := row.Scan(&ec.ID, &ec.MemberSpeciesIDs, &linksJSON, &ec.FetchedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEvolutionChainNotFound
		}
		return nil, fmt.Errorf("failed to query evolution chain %d: %w", id, err)
	}
	if len(linksJSON) > 0 {
		if err := json.Unmarshal(linksJSON, &ec.Links); err != nil {
			return nil, fmt.Errorf("failed to unmarshal evolution chain links %d: %w", id, err)
		}
	}
	return &ec, nil
}

func (r *postgresRepository) UpsertEvolutionChain(ctx context.Context, ec *EvolutionChain) error {
	now := time.Now()
	if ec.FetchedAt.IsZero() {
		ec.FetchedAt = now
	}

	linksJSON, err := json.Marshal(ec.Links)
	if err != nil {
		return fmt.Errorf("failed to marshal evolution chain links: %w", err)
	}
	if ec.Links == nil {
		linksJSON = []byte("[]") // avoid storing jsonb null
	}

	query := `
		INSERT INTO evolution_chain (id, member_species_ids, links, fetched_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			member_species_ids = EXCLUDED.member_species_ids,
			links = EXCLUDED.links,
			fetched_at = EXCLUDED.fetched_at
	`
	_, err = r.db.Exec(ctx, query, ec.ID, ec.MemberSpeciesIDs, linksJSON, ec.FetchedAt)
	if err != nil {
		return fmt.Errorf("failed to upsert evolution chain %d: %w", ec.ID, err)
	}
	return nil
}

func (r *postgresRepository) CandidatePool(ctx context.Context, filter PoolFilter) ([]*Pokemon, error) {
	// Deliberately omits abilities and raw_json: the enemy selector only needs
	// identity, stats and evolution-chain info. Selecting the multi-KB raw
	// payloads for every row made each pick a full multi-MB table scan, which
	// slowed 5v5 battle starts (5 sequential picks) past client timeouts.
	query := `
		SELECT id, name, species_id, evolution_chain_id, generation, types,
		       base_stats, COALESCE(sprite_url, ''), fetched_at, updated_at
		FROM pokemon
	`
	var args []any

	if filter.Generation != nil {
		query += " WHERE generation = $1"
		args = append(args, *filter.Generation)
	}
	query += " ORDER BY id ASC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query candidate pool: %w", err)
	}
	defer rows.Close()

	var result []*Pokemon
	for rows.Next() {
		var p Pokemon
		var baseStatsJSON []byte

		if err := rows.Scan(
			&p.ID, &p.Name, &p.SpeciesID, &p.EvolutionChainID, &p.Generation,
			&p.Types, &baseStatsJSON, &p.SpriteURL,
			&p.FetchedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan candidate pokemon row: %w", err)
		}

		if err := json.Unmarshal(baseStatsJSON, &p.BaseStats); err != nil {
			return nil, fmt.Errorf("failed to unmarshal base_stats: %w", err)
		}

		result = append(result, &p)
	}

	return result, rows.Err()
}
