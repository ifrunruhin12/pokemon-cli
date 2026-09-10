package pokemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"pokemon-cli/internal/middleware"

	"golang.org/x/sync/singleflight"
)

type PokemonService interface {
	GetByID(ctx context.Context, id int) (*Pokemon, error)
	GetByName(ctx context.Context, name string) (*Pokemon, error)
	EnsureEvolutionChain(ctx context.Context, speciesID int, optionalChainURL string) (int, error)
	GetRandomCard(ctx context.Context, allowSpecial bool) (Card, error)
	// GetEvolutionForLevel returns the Pokemon the given pokemon (by pokemon ID)
	// evolves into at the given level, or nil if no level-up evolution applies.
	GetEvolutionForLevel(ctx context.Context, pokemonID int, level int) (*Pokemon, error)
}

type service struct {
	cache  Cache
	repo   Repository
	client PokeAPIClient
	sf     singleflight.Group
}

func NewService(cache Cache, repo Repository, client PokeAPIClient) PokemonService {
	return &service{
		cache:  cache,
		repo:   repo,
		client: client,
	}
}

func (s *service) GetByID(ctx context.Context, id int) (*Pokemon, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid pokemon id: %d", id)
	}

	key := fmt.Sprintf("id:%d", id)

	v, err, _ := s.sf.Do(key, func() (any, error) {
		// 1. Redis hot cache
		if s.cache != nil {
			if p, err := s.cache.GetPokemon(ctx, id); err == nil && p != nil {
				slog.Debug("pokemon cache hit: Redis", "id", id, "name", p.Name)
				middleware.PokemonFetchTotal.WithLabelValues("redis").Inc()
				return p, nil
			}
		}

		// 2. PostgreSQL durable cache
		if s.repo != nil {
			if p, err := s.repo.GetPokemon(ctx, id); err == nil && p != nil {
				slog.Debug("pokemon cache hit: Postgres", "id", id, "name", p.Name)
				middleware.PokemonFetchTotal.WithLabelValues("postgres").Inc()
				if s.cache != nil {
					_ = s.cache.SetPokemon(ctx, p) // backfill Redis
				}
				return p, nil
			}
		}

		// 3. PokéAPI cold path
		if s.client == nil {
			return nil, fmt.Errorf("pokeapi client is not configured for cold fetch of id %d", id)
		}
		slog.Info("pokemon cache miss: fetching from PokeAPI", "id", id)
		middleware.PokemonFetchTotal.WithLabelValues("pokeapi").Inc()

		rawPokemon, err := s.client.FetchPokemonRaw(ctx, strconv.Itoa(id))
		if err != nil {
			return nil, fmt.Errorf("failed to fetch raw pokemon %d: %w", id, err)
		}

		rawSpecies, err := s.client.FetchSpeciesRaw(ctx, strconv.Itoa(id))
		if err != nil {
			return nil, fmt.Errorf("failed to fetch raw species %d: %w", id, err)
		}

		p, evolutionChainURL, err := normalizePokemon(rawPokemon, rawSpecies)
		if err != nil {
			return nil, fmt.Errorf("failed to normalize pokemon %d: %w", id, err)
		}

		evoChainID, err := s.EnsureEvolutionChain(ctx, p.SpeciesID, evolutionChainURL)
		if err != nil {
			// Non-fatal fallback for evolution chain if PokéAPI evolution-chain fails
			evoChainID = p.SpeciesID
		}
		p.EvolutionChainID = evoChainID

		if s.repo != nil {
			if err := s.repo.UpsertPokemon(ctx, p); err != nil {
				return nil, fmt.Errorf("failed to save pokemon %d to db: %w", id, err)
			}
			slog.Info("pokemon saved to Postgres", "id", id, "name", p.Name)
		}

		if s.cache != nil {
			_ = s.cache.SetPokemon(ctx, p)
			slog.Debug("pokemon saved to Redis", "id", id, "name", p.Name)
		}

		return p, nil
	})

	if err != nil {
		return nil, err
	}
	return v.(*Pokemon), nil
}

func (s *service) GetByName(ctx context.Context, name string) (*Pokemon, error) {
	cleanName := strings.TrimSpace(strings.ToLower(name))
	if cleanName == "" {
		return nil, fmt.Errorf("pokemon name cannot be empty")
	}

	// If numeric name passed in, redirect to GetByID
	if id, err := strconv.Atoi(cleanName); err == nil {
		return s.GetByID(ctx, id)
	}

	// 1. Check Redis name -> id index
	if s.cache != nil {
		if id, err := s.cache.GetIDByName(ctx, cleanName); err == nil && id > 0 {
			return s.GetByID(ctx, id)
		}
	}

	// 2. Check PostgreSQL by name
	if s.repo != nil {
		if p, err := s.repo.GetPokemonByName(ctx, cleanName); err == nil && p != nil {
			if s.cache != nil {
				_ = s.cache.SetPokemon(ctx, p)
			}
			return p, nil
		}
	}

	// 3. Cold path: Fetch raw Pokémon JSON by name to discover ID
	if s.client == nil {
		return nil, fmt.Errorf("pokeapi client not configured to resolve name %s", cleanName)
	}

	rawPokemon, err := s.client.FetchPokemonRaw(ctx, cleanName)
	if err != nil {
		return nil, fmt.Errorf("pokemon %q not found: %w", cleanName, err)
	}

	var meta struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(rawPokemon, &meta); err != nil || meta.ID <= 0 {
		return nil, fmt.Errorf("failed to parse id from raw pokemon payload for %s", cleanName)
	}

	// Delegate to GetByID to execute singleflight & write-through logic cleanly
	return s.GetByID(ctx, meta.ID)
}

func (s *service) EnsureEvolutionChain(ctx context.Context, speciesID int, optionalChainURL string) (int, error) {
	key := fmt.Sprintf("evochain_species:%d", speciesID)

	v, err, _ := s.sf.Do(key, func() (any, error) {
		var chainID int

		if optionalChainURL != "" {
			chainID, _ = parseEvolutionChainIDFromURL(optionalChainURL)
		}

		if chainID <= 0 && s.client != nil {
			// Fetch species payload to obtain evolution_chain.url
			rawSpecies, err := s.client.FetchSpeciesRaw(ctx, strconv.Itoa(speciesID))
			if err == nil {
				var speciesPayload struct {
					EvolutionChain struct {
						URL string `json:"url"`
					} `json:"evolution_chain"`
				}
				if err := json.Unmarshal(rawSpecies, &speciesPayload); err == nil {
					chainID, _ = parseEvolutionChainIDFromURL(speciesPayload.EvolutionChain.URL)
				}
			}
		}

		if chainID <= 0 {
			chainID = speciesID // Fallback: use species ID as chain ID
		}

		// 1. Check Redis cache for evolution chain
		if s.cache != nil {
			if ec, err := s.cache.GetEvolutionChain(ctx, chainID); err == nil && ec != nil {
				return ec.ID, nil
			}
		}

		// 2. Check PostgreSQL durable storage for evolution chain
		if s.repo != nil {
			if ec, err := s.repo.GetEvolutionChain(ctx, chainID); err == nil && ec != nil {
				if s.cache != nil {
					_ = s.cache.SetEvolutionChain(ctx, ec)
				}
				return ec.ID, nil
			}
		}

		// 3. Cold path: Fetch evolution chain from PokéAPI
		if s.client != nil {
			rawChain, err := s.client.FetchEvolutionChainRaw(ctx, chainID)
			if err == nil {
				links, memberIDs, err := ExtractEvolutionLinks(rawChain)
				if err == nil {
					ec := &EvolutionChain{
						ID:               chainID,
						MemberSpeciesIDs: memberIDs,
						Links:            links,
						FetchedAt:        time.Now(),
					}
					if s.repo != nil {
						_ = s.repo.UpsertEvolutionChain(ctx, ec)
					}
					if s.cache != nil {
						_ = s.cache.SetEvolutionChain(ctx, ec)
					}
					return chainID, nil
				}
			}
		}

		// Fallback evolution chain record
		ec := &EvolutionChain{
			ID:               chainID,
			MemberSpeciesIDs: []int{speciesID},
			FetchedAt:        time.Now(),
		}
		if s.repo != nil {
			_ = s.repo.UpsertEvolutionChain(ctx, ec)
		}
		if s.cache != nil {
			_ = s.cache.SetEvolutionChain(ctx, ec)
		}

		return chainID, nil
	})

	if err != nil {
		return speciesID, err
	}
	return v.(int), nil
}

// loadEvolutionChain returns the evolution chain by ID from cache, then
// PostgreSQL, then a cold PokéAPI fetch. Chains stored before the links column
// existed may have members but no links; those are refreshed once from PokéAPI
// so evolution data self-heals over time.
func (s *service) loadEvolutionChain(ctx context.Context, chainID int) *EvolutionChain {
	if chainID <= 0 {
		return nil
	}

	usable := func(ec *EvolutionChain) bool {
		return ec != nil && (len(ec.Links) > 0 || len(ec.MemberSpeciesIDs) <= 1)
	}

	var stale *EvolutionChain // multi-member chain cached before links existed

	if s.cache != nil {
		if ec, err := s.cache.GetEvolutionChain(ctx, chainID); err == nil {
			if usable(ec) {
				return ec
			}
			stale = ec
		}
	}

	if s.repo != nil {
		if ec, err := s.repo.GetEvolutionChain(ctx, chainID); err == nil {
			if usable(ec) {
				if s.cache != nil {
					_ = s.cache.SetEvolutionChain(ctx, ec)
				}
				return ec
			}
			stale = ec
		}
	}

	// Cold refresh is the only path that can recover missing links.
	// On failure, fall back to the stale chain (no evolution, but no error).
	if s.client != nil {
		if rawChain, err := s.client.FetchEvolutionChainRaw(ctx, chainID); err == nil {
			if links, memberIDs, err := ExtractEvolutionLinks(rawChain); err == nil {
				ec := &EvolutionChain{
					ID:               chainID,
					MemberSpeciesIDs: memberIDs,
					Links:            links,
					FetchedAt:        time.Now(),
				}
				if s.repo != nil {
					_ = s.repo.UpsertEvolutionChain(ctx, ec)
				}
				if s.cache != nil {
					_ = s.cache.SetEvolutionChain(ctx, ec)
				}
				return ec
			}
		}
	}

	return stale
}

// GetEvolutionForLevel returns the target Pokemon if the given pokemon has a
// level-up evolution whose minimum level has been reached. Branching chains
// (e.g. Eevee) resolve deterministically to the first qualifying link.
func (s *service) GetEvolutionForLevel(ctx context.Context, pokemonID int, level int) (*Pokemon, error) {
	p, err := s.GetByID(ctx, pokemonID)
	if err != nil {
		return nil, fmt.Errorf("failed to load pokemon %d for evolution check: %w", pokemonID, err)
	}

	chain := s.loadEvolutionChain(ctx, p.EvolutionChainID)
	if chain == nil {
		return nil, nil
	}

	link := PickEvolutionLink(chain.Links, p.SpeciesID, level)
	if link == nil {
		return nil, nil
	}

	// Pokemon IDs and species IDs align for the base forms this game stores.
	target, err := s.GetByID(ctx, link.ToSpeciesID)
	if err != nil {
		return nil, fmt.Errorf("failed to load evolution target %d: %w", link.ToSpeciesID, err)
	}
	return target, nil
}

func (s *service) GetRandomCard(ctx context.Context, allowSpecial bool) (Card, error) {
	mythicalOdds := 0.0001
	legendaryOdds := 0.0001
	maxRetries := 5

	for i := 0; i < maxRetries; i++ {
		roll := rand.Float64()
		var targetID int

		if allowSpecial && roll < mythicalOdds {
			// Mythical list IDs
			targetID = 151 // Mew
		} else if allowSpecial && roll < mythicalOdds+legendaryOdds {
			// Legendary list IDs
			targetID = 150 // Mewtwo
		} else {
			targetID = rand.Intn(649) + 1
		}

		p, err := s.GetByID(ctx, targetID)
		if err != nil {
			continue
		}
		return p.ToCard(), nil
	}

	// Fallback to Pikachu (#25)
	p, err := s.GetByID(ctx, 25)
	if err == nil {
		return p.ToCard(), nil
	}

	return Card{}, fmt.Errorf("failed to fetch random pokemon card: %w", err)
}

// Internal helper to normalize raw PokéAPI responses into domain Pokemon struct
func normalizePokemon(rawPokemon []byte, rawSpecies []byte) (*Pokemon, string, error) {
	var pData struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Stats []struct {
			BaseStat int `json:"base_stat"`
			Stat     struct {
				Name string `json:"name"`
			} `json:"stat"`
		} `json:"stats"`
		Types []struct {
			Type struct {
				Name string `json:"name"`
			} `json:"type"`
		} `json:"types"`
		Abilities json.RawMessage `json:"abilities"`
		Sprites   struct {
			FrontDefault string `json:"front_default"`
			Other        struct {
				OfficialArtwork struct {
					FrontDefault string `json:"front_default"`
				} `json:"official-artwork"`
			} `json:"other"`
		} `json:"sprites"`
	}

	if err := json.Unmarshal(rawPokemon, &pData); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal raw pokemon: %w", err)
	}

	var sData struct {
		ID             int `json:"id"`
		EvolutionChain struct {
			URL string `json:"url"`
		} `json:"evolution_chain"`
		Generation struct {
			Name string `json:"name"`
		} `json:"generation"`
	}
	_ = json.Unmarshal(rawSpecies, &sData)

	var baseStats BaseStats
	for _, st := range pData.Stats {
		switch st.Stat.Name {
		case "hp":
			baseStats.HP = st.BaseStat
		case "attack":
			baseStats.Attack = st.BaseStat
		case "defense":
			baseStats.Defense = st.BaseStat
		case "special-attack":
			baseStats.SpecialAttack = st.BaseStat
		case "special-defense":
			baseStats.SpecialDefense = st.BaseStat
		case "speed":
			baseStats.Speed = st.BaseStat
		}
	}

	var types []string
	for _, t := range pData.Types {
		types = append(types, t.Type.Name)
	}

	spriteURL := pData.Sprites.Other.OfficialArtwork.FrontDefault
	if spriteURL == "" {
		spriteURL = pData.Sprites.FrontDefault
	}

	speciesID := sData.ID
	if speciesID <= 0 {
		speciesID = pData.ID
	}

	genNum := parseGenerationNumber(sData.Generation.Name)

	return &Pokemon{
		ID:         pData.ID,
		Name:       strings.ToLower(pData.Name),
		SpeciesID:  speciesID,
		Generation: genNum,
		Types:      types,
		BaseStats:  baseStats,
		Abilities:  pData.Abilities,
		SpriteURL:  spriteURL,
		RawJSON:    rawPokemon,
		FetchedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, sData.EvolutionChain.URL, nil
}
