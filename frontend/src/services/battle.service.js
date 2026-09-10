import api from './api';

/**
 * Battle Service
 * Handles all battle-related API calls
 */

/**
 * Start a new battle
 * @param {string} mode - Battle mode ('1v1' or '5v5')
 * @returns {Promise<Object>} Battle state
 */
export const startBattle = async (mode = '5v5') => {
  // Battle start builds the AI team server-side; 5v5 can legitimately take
  // longer than the global 10s axios timeout, so give it extra headroom.
  const response = await api.post('/api/battle/start', { mode }, { timeout: 30000 });
  return response.data;
};

/**
 * Submit a move in the current battle
 * @param {string} battleId - Battle session ID
 * @param {string} move - Move type ('attack', 'defend', 'pass', 'sacrifice', 'surrender')
 * @param {number} moveIdx - Move index (required for 'attack')
 * @returns {Promise<Object>} Battle result with updated state
 */
export const submitMove = async (battleId, move, moveIdx = null) => {
  const payload = {
    battle_id: battleId,
    move: move
  };
  
  // Only include move_idx if it's provided (for attack moves)
  if (moveIdx !== null && moveIdx !== undefined) {
    payload.move_idx = moveIdx;
  }
  
  const response = await api.post('/api/battle/move', payload);
  return response.data;
};

/**
 * Get current battle state
 * @param {string} battleId - Battle session ID
 * @returns {Promise<Object>} Current battle state
 */
export const getBattleState = async (battleId) => {
  const response = await api.get(`/api/battle/state?battle_id=${battleId}`);
  return response.data;
};

/**
 * Switch active Pokemon
 * @param {string} battleId - Battle session ID
 * @param {number} newIdx - Index of Pokemon to switch to
 * @returns {Promise<Object>} Updated battle state
 */
export const switchPokemon = async (battleId, newIdx) => {
  const response = await api.post('/api/battle/switch', {
    battle_id: battleId,
    new_idx: newIdx
  });
  return response.data;
};

/**
 * Select reward Pokemon after 5v5 victory
 * @param {string} battleId - Battle session ID
 * @param {number} pokemonIdx - Index of AI Pokemon to select
 * @returns {Promise<Object>} Selected Pokemon card
 */
export const selectReward = async (battleId, pokemonIdx) => {
  const response = await api.post('/api/battle/select-reward', {
    battle_id: battleId,
    pokemon_index: pokemonIdx
  });
  return response.data;
};

export default {
  startBattle,
  submitMove,
  getBattleState,
  switchPokemon,
  selectReward
};
