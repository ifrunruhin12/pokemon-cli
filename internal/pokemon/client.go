package pokemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type PokeAPIClient interface {
	FetchPokemonRaw(ctx context.Context, idOrName string) ([]byte, error)
	FetchSpeciesRaw(ctx context.Context, idOrName string) ([]byte, error)
	FetchEvolutionChainRaw(ctx context.Context, chainID int) ([]byte, error)
}

type pokeAPIClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewPokeAPIClient(baseURL string, timeout time.Duration) PokeAPIClient {
	if baseURL == "" {
		baseURL = "https://pokeapi.co/api/v2"
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &pokeAPIClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *pokeAPIClient) FetchPokemonRaw(ctx context.Context, idOrName string) ([]byte, error) {
	target := fmt.Sprintf("%s/pokemon/%s", c.baseURL, url.PathEscape(strings.ToLower(idOrName)))
	return c.get(ctx, target)
}

func (c *pokeAPIClient) FetchSpeciesRaw(ctx context.Context, idOrName string) ([]byte, error) {
	target := fmt.Sprintf("%s/pokemon-species/%s", c.baseURL, url.PathEscape(strings.ToLower(idOrName)))
	return c.get(ctx, target)
}

func (c *pokeAPIClient) FetchEvolutionChainRaw(ctx context.Context, chainID int) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/evolution-chain/%d", c.baseURL, chainID)
	return c.get(ctx, endpoint)
}

func (c *pokeAPIClient) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed for %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("resource not found at %s", endpoint)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, endpoint)
	}

	var data json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode json response: %w", err)
	}
	return data, nil
}

// Helpers for parsing PokéAPI payloads

func parseEvolutionChainIDFromURL(rawURL string) (int, error) {
	trimmed := strings.TrimRight(rawURL, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid evolution chain url: %s", rawURL)
	}
	return strconv.Atoi(parts[len(parts)-1])
}

func parseGenerationNumber(genName string) int {
	// generation-i -> 1, generation-ii -> 2, etc.
	romanMap := map[string]int{
		"generation-i":    1,
		"generation-ii":   2,
		"generation-iii":  3,
		"generation-iv":   4,
		"generation-v":    5,
		"generation-vi":   6,
		"generation-vii":  7,
		"generation-viii": 8,
		"generation-ix":   9,
	}
	if val, ok := romanMap[strings.ToLower(genName)]; ok {
		return val
	}
	// fallback if it's numeric or unknown
	parts := strings.Split(genName, "-")
	if len(parts) > 1 {
		if num, err := strconv.Atoi(parts[1]); err == nil {
			return num
		}
	}
	return 1
}

// ExtractMemberSpeciesIDs extracts all species IDs recursively from evolution chain JSON
func ExtractMemberSpeciesIDs(chainJSON []byte) ([]int, error) {
	var payload struct {
		Chain struct {
			Species struct {
				URL string `json:"url"`
			} `json:"species"`
			EvolvesTo []json.RawMessage `json:"evolves_to"`
		} `json:"chain"`
	}

	if err := json.Unmarshal(chainJSON, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal evolution chain payload: %w", err)
	}

	var memberIDs []int
	var walk func(rawNode json.RawMessage)
	walk = func(rawNode json.RawMessage) {
		var node struct {
			Species struct {
				URL string `json:"url"`
			} `json:"species"`
			EvolvesTo []json.RawMessage `json:"evolves_to"`
		}
		if err := json.Unmarshal(rawNode, &node); err != nil {
			return
		}
		if id, err := parseEvolutionChainIDFromURL(node.Species.URL); err == nil {
			memberIDs = append(memberIDs, id)
		}
		for _, child := range node.EvolvesTo {
			walk(child)
		}
	}

	// Process root
	if id, err := parseEvolutionChainIDFromURL(payload.Chain.Species.URL); err == nil {
		memberIDs = append(memberIDs, id)
	}
	for _, child := range payload.Chain.EvolvesTo {
		walk(child)
	}

	return memberIDs, nil
}

// evolutionChainNode mirrors one node of the PokéAPI evolution chain tree.
type evolutionChainNode struct {
	Species struct {
		URL string `json:"url"`
	} `json:"species"`
	EvolutionDetails []evolutionDetail    `json:"evolution_details"`
	EvolvesTo        []evolutionChainNode `json:"evolves_to"`
}

// evolutionDetail holds the trigger conditions for a single evolution edge.
type evolutionDetail struct {
	MinLevel *int `json:"min_level"`
	Trigger  struct {
		Name string `json:"name"`
	} `json:"trigger"`
}

// ExtractEvolutionLinks walks the evolution chain tree and returns every edge
// with its trigger details. Each evolves_to entry may carry multiple
// evolution_details; we keep the first one that has a level-up trigger, or the
// first detail overall so the edge is not lost.
func ExtractEvolutionLinks(chainJSON []byte) ([]EvolutionLink, []int, error) {
	var payload struct {
		Chain evolutionChainNode `json:"chain"`
	}
	if err := json.Unmarshal(chainJSON, &payload); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal evolution chain payload: %w", err)
	}

	var links []EvolutionLink
	var memberIDs []int

	speciesID := func(n evolutionChainNode) (int, bool) {
		id, err := parseEvolutionChainIDFromURL(n.Species.URL)
		return id, err == nil
	}

	var walk func(n evolutionChainNode)
	walk = func(n evolutionChainNode) {
		if fromID, ok := speciesID(n); ok {
			memberIDs = append(memberIDs, fromID)
		}
		for _, child := range n.EvolvesTo {
			toID, ok := speciesID(child)
			if !ok {
				continue
			}

			fromID, hasFrom := speciesID(n)
			if hasFrom {
				link := EvolutionLink{FromSpeciesID: fromID, ToSpeciesID: toID}
				for _, d := range child.EvolutionDetails {
					if d.Trigger.Name != "" {
						link.Trigger = d.Trigger.Name
					}
					if d.MinLevel != nil {
						link.MinLevel = *d.MinLevel
					}
					if d.Trigger.Name == "level-up" {
						break // prefer the level-up detail when present
					}
				}
				links = append(links, link)
			}
			walk(child)
		}
	}
	walk(payload.Chain)

	return links, memberIDs, nil
}
