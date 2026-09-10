package pokemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Minimal but structurally faithful excerpt of PokéAPI's evolution-chain
// payload for the Charmander family (chain id 2).
const charmanderChainJSON = `{
  "id": 2,
  "chain": {
    "species": { "name": "charmander", "url": "https://pokeapi.co/api/v2/pokemon-species/4/" },
    "evolution_details": [],
    "evolves_to": [
      {
        "species": { "name": "charmeleon", "url": "https://pokeapi.co/api/v2/pokemon-species/5/" },
        "evolution_details": [
          { "min_level": 16, "trigger": { "name": "level-up", "url": "https://pokeapi.co/api/v2/evolution-trigger/1/" } }
        ],
        "evolves_to": [
          {
            "species": { "name": "charizard", "url": "https://pokeapi.co/api/v2/pokemon-species/6/" },
            "evolution_details": [
              { "min_level": 36, "trigger": { "name": "level-up", "url": "https://pokeapi.co/api/v2/evolution-trigger/1/" } }
            ],
            "evolves_to": []
          }
        ]
      }
    ]
  }
}`

// Eevee-like branching chain with non-level triggers mixed in.
const branchingChainJSON = `{
  "id": 67,
  "chain": {
    "species": { "name": "eevee", "url": "https://pokeapi.co/api/v2/pokemon-species/133/" },
    "evolution_details": [],
    "evolves_to": [
      {
        "species": { "name": "jolteon", "url": "https://pokeapi.co/api/v2/pokemon-species/135/" },
        "evolution_details": [
          { "min_level": null, "trigger": { "name": "use-item", "url": "https://pokeapi.co/api/v2/evolution-trigger/3/" } }
        ],
        "evolves_to": []
      },
      {
        "species": { "name": "espeon", "url": "https://pokeapi.co/api/v2/pokemon-species/196/" },
        "evolution_details": [
          { "min_level": null, "trigger": { "name": "level-up", "url": "https://pokeapi.co/api/v2/evolution-trigger/1/" }, "time_of_day": "day" }
        ],
        "evolves_to": []
      }
    ]
  }
}`

func TestExtractEvolutionLinksCharmanderFamily(t *testing.T) {
	links, members, err := ExtractEvolutionLinks([]byte(charmanderChainJSON))
	require.NoError(t, err)

	assert.Equal(t, []int{4, 5, 6}, members)
	assert.Equal(t, []EvolutionLink{
		{FromSpeciesID: 4, ToSpeciesID: 5, MinLevel: 16, Trigger: "level-up"},
		{FromSpeciesID: 5, ToSpeciesID: 6, MinLevel: 36, Trigger: "level-up"},
	}, links)
}

func TestExtractEvolutionLinksBranchingChain(t *testing.T) {
	links, members, err := ExtractEvolutionLinks([]byte(branchingChainJSON))
	require.NoError(t, err)

	assert.Equal(t, []int{133, 135, 196}, members)
	require.Len(t, links, 2)
	assert.Equal(t, EvolutionLink{FromSpeciesID: 133, ToSpeciesID: 135, Trigger: "use-item"}, links[0])
	// level-up edge without a min_level is kept but unusable for our purposes
	assert.Equal(t, EvolutionLink{FromSpeciesID: 133, ToSpeciesID: 196, Trigger: "level-up"}, links[1])
}

func TestPickEvolutionLink(t *testing.T) {
	links := []EvolutionLink{
		{FromSpeciesID: 4, ToSpeciesID: 5, MinLevel: 16, Trigger: "level-up"},
		{FromSpeciesID: 5, ToSpeciesID: 6, MinLevel: 36, Trigger: "level-up"},
		{FromSpeciesID: 133, ToSpeciesID: 135, Trigger: "use-item"},
	}

	t.Run("below threshold", func(t *testing.T) {
		assert.Nil(t, PickEvolutionLink(links, 4, 15))
	})

	t.Run("at threshold", func(t *testing.T) {
		got := PickEvolutionLink(links, 4, 16)
		require.NotNil(t, got)
		assert.Equal(t, 5, got.ToSpeciesID)
	})

	t.Run("second stage", func(t *testing.T) {
		got := PickEvolutionLink(links, 5, 40)
		require.NotNil(t, got)
		assert.Equal(t, 6, got.ToSpeciesID)
	})

	t.Run("final form has no further evolution", func(t *testing.T) {
		assert.Nil(t, PickEvolutionLink(links, 6, 50))
	})

	t.Run("non-level-up triggers are ignored", func(t *testing.T) {
		assert.Nil(t, PickEvolutionLink(links, 133, 50))
	})
}
