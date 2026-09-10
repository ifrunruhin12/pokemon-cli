package pokemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticPokemonService struct {
	pokemon *Pokemon
}

func (s staticPokemonService) GetByID(context.Context, int) (*Pokemon, error) {
	return s.pokemon, nil
}

func (s staticPokemonService) GetByName(context.Context, string) (*Pokemon, error) {
	return s.pokemon, nil
}

func (s staticPokemonService) EnsureEvolutionChain(context.Context, int, string) (int, error) {
	return 0, nil
}

func (s staticPokemonService) GetRandomCard(context.Context, bool) (Card, error) {
	return Card{}, nil
}

func (s staticPokemonService) GetEvolutionForLevel(context.Context, int, int) (*Pokemon, error) {
	return nil, nil
}

func TestFetchPokemonPreservesServiceRawData(t *testing.T) {
	moveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"thunderbolt","power":90,"type":{"name":"electric"}}`))
	}))
	defer moveServer.Close()

	rawJSON, err := json.Marshal(map[string]any{
		"name": "pikachu",
		"stats": []map[string]any{
			{"base_stat": 35, "stat": map[string]string{"name": "hp"}},
			{"base_stat": 55, "stat": map[string]string{"name": "attack"}},
			{"base_stat": 40, "stat": map[string]string{"name": "defense"}},
			{"base_stat": 50, "stat": map[string]string{"name": "special-attack"}},
			{"base_stat": 50, "stat": map[string]string{"name": "special-defense"}},
			{"base_stat": 90, "stat": map[string]string{"name": "speed"}},
		},
		"moves": []map[string]any{{"move": map[string]string{"name": "thunderbolt", "url": moveServer.URL}}},
	})
	require.NoError(t, err)

	SetDefaultService(staticPokemonService{pokemon: &Pokemon{RawJSON: rawJSON}})
	defer SetDefaultService(nil)

	raw, moves, err := FetchPokemon("pikachu")
	require.NoError(t, err)
	assert.Len(t, raw.Stats, 6)
	assert.Len(t, raw.Moves, 1)
	require.Len(t, moves, 1)
	assert.Equal(t, "thunderbolt", moves[0].Name)
	assert.Equal(t, 90, moves[0].Power)
}

func TestPokemonToCardMatchesBuilderStatsAndRarity(t *testing.T) {
	assert.Equal(t, BuildCardFromPokemon(RawPokeAPIPokemon{}, nil).HP, (&Pokemon{}).ToCard().HP)

	for _, name := range []string{"mew", "mewtwo"} {
		t.Run(name, func(t *testing.T) {
			raw := RawPokeAPIPokemon{Name: name, Stats: []Stat{
				stat("hp", 35),
				stat("attack", 55),
				stat("defense", 40),
				stat("speed", 90),
			}}
			domain := &Pokemon{
				Name: name,
				BaseStats: BaseStats{
					HP: 35, Attack: 55, Defense: 40, Speed: 90,
				},
			}

			built := BuildCardFromPokemon(raw, nil)
			converted := domain.ToCard()
			assert.Equal(t,
				[]int{built.HP, built.HPMax, built.Attack, built.Defense, built.Speed},
				[]int{converted.HP, converted.HPMax, converted.Attack, converted.Defense, converted.Speed},
			)
			assert.Equal(t, built.IsLegendary, converted.IsLegendary)
			assert.Equal(t, built.IsMythical, converted.IsMythical)
		})
	}
}

func stat(name string, value int) Stat {
	return Stat{
		BaseSt: value,
		StName: struct {
			Name string `json:"name"`
		}{Name: name},
	}
}
