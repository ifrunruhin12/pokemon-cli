import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { BattleArena, BattleEntryAnimation } from '../components/battle';
import { startBattle, submitMove, switchPokemon, selectReward } from '../services/battle.service';
import tokenService from '../services/token.service';
import { motion, AnimatePresence } from 'framer-motion';

export default function Battle() {
  const navigate = useNavigate();
  const [battleState, setBattleState] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [battleMode, setBattleMode] = useState(null);
  const [showEntryAnimation, setShowEntryAnimation] = useState(false);
  const [tokenData, setTokenData] = useState(null);
  const [loadingTokens, setLoadingTokens] = useState(true);
  const [battleStartUncertain, setBattleStartUncertain] = useState(false);

  // Transform backend response to frontend format
  const transformBattleState = (response) => {
    console.log('Transforming battle state:', response);
    if (!response) {
      console.error('Invalid response:', response);
      return null;
    }
    
    // Transform Pokemon data from backend format
    const transformPokemon = (pokemon) => {
      if (!pokemon) return null;
      
      // Handle both snake_case (from backend) and camelCase formats
      return {
        name: pokemon.name || pokemon.Name,
        pokemon_name: pokemon.name || pokemon.Name,
        hp: pokemon.hp !== undefined ? pokemon.hp : pokemon.HP,
        hp_max: pokemon.hp_max !== undefined ? pokemon.hp_max : pokemon.HPMax,
        stamina: pokemon.stamina !== undefined ? pokemon.stamina : pokemon.Stamina,
        stamina_max: pokemon.stamina_max !== undefined ? pokemon.stamina_max : pokemon.StaminaMax,
        defense: pokemon.defense !== undefined ? pokemon.defense : pokemon.Defense,
        attack: pokemon.attack !== undefined ? pokemon.attack : pokemon.Attack,
        speed: pokemon.speed !== undefined ? pokemon.speed : pokemon.Speed,
        moves: pokemon.moves || pokemon.Moves || [],
        types: pokemon.types || pokemon.Types || [],
        sprite: pokemon.sprite || pokemon.Sprite,
        level: pokemon.level || pokemon.Level || 1,
        xp: pokemon.xp || pokemon.XP || 0,
        is_legendary: pokemon.is_legendary || pokemon.IsLegendary || false,
        is_mythical: pokemon.is_mythical || pokemon.IsMythical || false,
        is_knocked_out: pokemon.is_knocked_out || pokemon.IsKnockedOut || false,
        is_face_down: pokemon.is_face_down || false
      };
    };
    
    // Handle both new flat response format and old nested format
    const data = response.state || response;
    const battleId = response.session || response.id || data.id;
    
    // Transform XP gains from backend format to frontend format
    const transformXPGains = (xpGains) => {
      if (!xpGains || xpGains.length === 0) return null;
      
      const pokemon_details = xpGains.map(gain => ({
        card_id: gain.card_id,
        name: gain.evolved ? gain.evolved_into : gain.pokemon_name,
        level: gain.new_level,
        xp_gained: gain.xp_gained,
        leveled_up: gain.leveled_up,
        evolved: gain.evolved || false,
        sprite: gain.new_sprite || null // Sprites of evolved forms
      }));
      
      const level_ups = xpGains
        .filter(gain => gain.leveled_up)
        .map(gain => ({
          name: gain.evolved ? gain.evolved_into : gain.pokemon_name,
          old_level: gain.old_level,
          new_level: gain.new_level,
          stat_increases: {
            hp: gain.new_hp - gain.old_hp,
            attack: gain.new_attack - gain.old_attack,
            defense: gain.new_defense - gain.old_defense,
            speed: gain.new_speed - gain.old_speed
          }
        }));
      
      const evolutions = xpGains
        .filter(gain => gain.evolved)
        .map(gain => ({
          from: gain.evolved_from,
          into: gain.evolved_into,
          level: gain.new_level,
          sprite: gain.new_sprite || null
        }));
      
      return {
        pokemon_details,
        level_ups: level_ups.length > 0 ? level_ups : undefined,
        evolutions: evolutions.length > 0 ? evolutions : undefined
      };
    };
    
    // Build rewards object
    const rewards = data.rewards || {};
    
    // Add coins earned
    if (data.coins_earned !== undefined) {
      rewards.coins_earned = data.coins_earned;
    }
    
    // Add XP gains
    if (data.xp_gains && data.xp_gains.length > 0) {
      const xpData = transformXPGains(data.xp_gains);
      if (xpData) {
        rewards.pokemon_details = xpData.pokemon_details;
        if (xpData.level_ups) {
          rewards.level_ups = xpData.level_ups;
        }
        if (xpData.evolutions) {
          rewards.evolutions = xpData.evolutions;
        }
      }
    }
    
    // Add newly unlocked achievements
    if (data.newly_unlocked_achievements && data.newly_unlocked_achievements.length > 0) {
      rewards.newly_unlocked_achievements = data.newly_unlocked_achievements;
    }
    
    const transformed = {
      id: battleId,
      mode: data.mode || data.BattleMode || '1v1',
      player_deck: (data.player_deck || data.Player?.Deck || []).map(transformPokemon),
      ai_deck: (data.ai_deck || data.AI?.Deck || []).map(transformPokemon),
      player_active_idx: data.player_active_idx !== undefined ? data.player_active_idx : (data.PlayerActiveIdx || 0),
      ai_active_idx: data.ai_active_idx !== undefined ? data.ai_active_idx : (data.AIActiveIdx || 0),
      turn_number: data.turn_number || data.TurnNumber || 1,
      round_number: data.round_number || data.Round || 1,
      whose_turn: data.whose_turn || data.turn?.WhoseTurn || 'player',
      battle_over: data.battle_over || data.BattleOver || false,
      winner: data.winner || (data.BattleOver ? (data.PlayerSurrendered ? 'ai' : 'player') : null),
      log: data.log || [],
      rewards: Object.keys(rewards).length > 0 ? rewards : undefined,
      player_sacrifice_count: data.player_sacrifice_count || 0,
      player_sacrifice_cost: data.player_sacrifice_cost ?? null,
    };
    
    console.log('Transformed state:', transformed);
    return transformed;
  };

  // Fetch token balance
  const fetchTokenBalance = async () => {
    setLoadingTokens(true);
    try {
      const data = await tokenService.getTokenBalance();
      setTokenData(data);
    } catch (error) {
      console.error('Failed to fetch token balance:', error);
      setError('Failed to load token information');
    } finally {
      setLoadingTokens(false);
    }
  };

  // Fetch token balance on mount and when returning from battle
  useEffect(() => {
    if (!battleState) {
      fetchTokenBalance();
    }
  }, [battleState]);

  // Start a new battle
  const handleStartBattle = async (mode) => {
    if (battleStartUncertain) return;

    setLoading(true);
    setError(null);
    try {
      const response = await startBattle(mode);
      const transformedState = transformBattleState(response);
      setBattleState(transformedState);
      setBattleMode(mode);
      setShowEntryAnimation(true);
    } catch (err) {
      // Timeout: the server may still complete the request in the background
      // (consuming a token and creating the battle), so tell the user honestly.
      // Checked first because the message contains the word "token".
      if (err.code === 'ECONNABORTED' || err.message?.includes('timeout')) {
        setBattleStartUncertain(true);
        setError('Battle start timed out. The battle may still have been created. Reload before trying again to avoid losing a token.');
        console.error('Battle start timed out:', err);
        fetchTokenBalance();
        return;
      }

      const errorMessage = err.response?.data?.error?.message || err.response?.data?.error || 'Failed to start battle';

      // Check if it's an insufficient tokens error
      if (errorMessage.includes('token') || errorMessage.includes('INSUFFICIENT_TOKENS')) {
        setError('insufficient_tokens');
      } else {
        setError(errorMessage);
      }
      console.error('Error starting battle:', err);

      // Refresh token balance after error
      fetchTokenBalance();
    } finally {
      setLoading(false);
    }
  };

  // Handle entry animation complete
  const handleEntryAnimationComplete = () => {
    console.log('Entry animation complete');
    setShowEntryAnimation(false);
  };
  
  // Auto-skip animation after 3 seconds as fallback
  useEffect(() => {
    if (showEntryAnimation) {
      const timer = setTimeout(() => {
        console.log('Auto-skipping animation');
        setShowEntryAnimation(false);
      }, 3000);
      return () => clearTimeout(timer);
    }
  }, [showEntryAnimation]);

  // Handle move submission
  const handleMove = async (move, moveIdx = null) => {
    if (!battleState?.id) {
      console.error('No battle ID found:', battleState);
      setError('Battle session not found. Please start a new battle.');
      return;
    }
    
    console.log('Submitting move:', { battleId: battleState.id, move, moveIdx });
    setLoading(true);
    setError(null); // Clear previous errors
    
    try {
      const result = await submitMove(battleState.id, move, moveIdx);
      console.log('Move result:', result);
      // Keep the same battle ID since the backend response might not include it
      const resultWithId = { ...result, id: result.id || battleState.id };
      const transformedState = transformBattleState(resultWithId);
      setBattleState(transformedState);
    } catch (err) {
      const errorMessage = err.response?.data?.error?.message 
        || err.response?.data?.error 
        || err.message 
        || 'Failed to submit move';
      setError(errorMessage);
      console.error('Error submitting move:', err);
    } finally {
      setLoading(false);
    }
  };

  // Handle Pokemon switching
  const handleSwitchPokemon = async (newIdx) => {
    if (!battleState?.id) {
      setError('Battle session not found. Please start a new battle.');
      return;
    }
    
    setLoading(true);
    setError(null); // Clear previous errors
    
    try {
      const result = await switchPokemon(battleState.id, newIdx);
      // Keep the same battle ID since the backend response might not include it
      const resultWithId = { ...result, id: result.id || battleState.id };
      const transformedState = transformBattleState(resultWithId);
      setBattleState(transformedState);
    } catch (err) {
      const errorMessage = err.response?.data?.error?.message 
        || err.response?.data?.error 
        || err.message 
        || 'Failed to switch Pokemon';
      setError(errorMessage);
      console.error('Error switching Pokemon:', err);
    } finally {
      setLoading(false);
    }
  };

  // Handle reward selection (5v5 victory)
  const handleSelectReward = async (pokemonIdx) => {
    if (!battleState?.id) {
      setError('Battle session not found. Please start a new battle.');
      return;
    }
    
    setLoading(true);
    setError(null); // Clear previous errors
    
    try {
      const result = await selectReward(battleState.id, pokemonIdx);
      console.log('Reward selected:', result);
      
      // Update battle state to mark reward as claimed
      // This will hide the reward selection UI and show action buttons
      setBattleState(prev => ({
        ...prev,
        reward_claimed: true
      }));
      
      // Show success message
      alert(`Successfully added ${result.card?.pokemon_name || 'Pokémon'} to your collection!`);
    } catch (err) {
      const errorMessage = err.response?.data?.error?.message 
        || err.response?.data?.error 
        || err.message 
        || 'Failed to select reward';
      setError(errorMessage);
      console.error('Error selecting reward:', err);
      throw err; // Re-throw to let BattleResult handle it
    } finally {
      setLoading(false);
    }
  };

  // Handle rematch
  const handleRematch = () => {
    if (battleMode) {
      handleStartBattle(battleMode);
    }
  };

  // Handle new battle
  const handleNewBattle = () => {
    setBattleState(null);
    setBattleMode(null);
  };

  // Handle return to menu
  const handleReturnToMenu = () => {
    navigate('/dashboard');
  };

  // Battle mode selection screen
  if (!battleState && !loading) {
    const tokens = tokenData?.game_tokens || 0;
    const hasNoTokens = tokens === 0;
    const hasLowTokens = tokens > 0 && tokens <= 2;
    const battleStartDisabled = hasNoTokens || battleStartUncertain;

    return (
      <div className="min-h-screen bg-gradient-to-br from-gray-900 via-gray-800 to-gray-900 flex items-center justify-center p-4">
        <motion.div
          initial={{ opacity: 0, scale: 0.9 }}
          animate={{ opacity: 1, scale: 1 }}
          className="max-w-2xl w-full"
        >
          <div className="bg-gray-800 rounded-2xl shadow-2xl p-8 border-2 border-gray-700">
            <h1 className="text-4xl font-bold text-white text-center mb-4">
              ⚔️ Battle Arena
            </h1>
            <p className="text-gray-400 text-center mb-8">
              Choose your battle mode and test your skills!
            </p>

            {/* Token Display */}
            {!loadingTokens && tokenData && (
              <div className="mb-6 bg-gray-700/50 rounded-lg p-4 border border-gray-600">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <span className="text-3xl">🎫</span>
                    <div>
                      <div className="text-white font-semibold text-lg">
                        {tokens > (tokenData?.daily_token_limit || 10) ? `${tokens} Tokens` : `${tokens}/${tokenData?.daily_token_limit || 10} Tokens`}
                      </div>
                      <div className="text-gray-400 text-sm">
                        Resets at {tokenData.reset_time} ({tokenData.time_until_reset})
                      </div>
                    </div>
                  </div>
                  <div className="text-right">
                    <div className="text-yellow-400 text-sm">
                      1v1 costs {tokenData?.token_cost_1v1 || 1} token{(tokenData?.token_cost_1v1 || 1) !== 1 ? 's' : ''} | 
                      5v5 costs {tokenData?.token_cost_5v5 || 2} token{(tokenData?.token_cost_5v5 || 2) !== 1 ? 's' : ''}
                    </div>
                  </div>
                </div>
              </div>
            )}

            {/* No Tokens Error */}
            {hasNoTokens && (
              <div className="bg-red-900/50 border border-red-500 text-red-200 px-4 py-3 rounded-lg mb-6">
                <div className="flex items-center gap-2 mb-2">
                  <span className="text-xl">⚠️</span>
                  <span className="font-semibold">No Tokens Available</span>
                </div>
                <p className="text-sm mb-3">
                  You don't have any tokens to start a battle. Purchase more tokens from the shop or wait for your daily reset.
                </p>
                <button
                  onClick={() => navigate('/shop')}
                  className="bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 rounded-lg text-sm font-semibold transition-colors"
                >
                  Go to Shop →
                </button>
              </div>
            )}

            {/* Low Tokens Warning */}
            {hasLowTokens && (
              <div className="bg-yellow-900/50 border border-yellow-500 text-yellow-200 px-4 py-3 rounded-lg mb-6">
                <div className="flex items-center gap-2">
                  <span className="text-xl">⚠️</span>
                  <span className="font-semibold">Low on Tokens!</span>
                </div>
                <p className="text-sm mt-1">
                  You only have {tokens} token{tokens !== 1 ? 's' : ''} remaining. Consider purchasing more from the shop.
                </p>
              </div>
            )}

            {/* Other Errors */}
            {error && error !== 'insufficient_tokens' && (
              <div className="bg-red-900/50 border border-red-500 text-red-200 px-4 py-3 rounded-lg mb-6">
                <p>{error}</p>
                {battleStartUncertain && (
                  <button
                    onClick={() => window.location.reload()}
                    className="mt-3 bg-red-600 hover:bg-red-500 text-white px-4 py-2 rounded-lg text-sm font-semibold transition-colors"
                  >
                    Reload battle status
                  </button>
                )}
              </div>
            )}

            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              {/* 1v1 Battle */}
              <motion.button
                whileHover={!battleStartDisabled ? { scale: 1.05, y: -5 } : {}}
                whileTap={!battleStartDisabled ? { scale: 0.95 } : {}}
                onClick={() => !battleStartDisabled && handleStartBattle('1v1')}
                disabled={battleStartDisabled}
                className={`rounded-xl p-6 shadow-lg transition-all ${
                  battleStartDisabled
                    ? 'bg-gray-700 text-gray-500 cursor-not-allowed opacity-50'
                    : 'bg-gradient-to-br from-blue-600 to-blue-700 hover:from-blue-500 hover:to-blue-600 text-white'
                }`}
              >
                <div className="text-5xl mb-4">⚔️</div>
                <h2 className="text-2xl font-bold mb-2">1v1 Battle</h2>
                <p className={`text-sm mb-4 ${battleStartDisabled ? 'text-gray-500' : 'text-blue-100'}`}>
                  Quick battle with one Pokemon
                </p>
                <div className={`font-semibold ${battleStartDisabled ? 'text-gray-500' : 'text-yellow-300'}`}>
                  💰 Win: 50 coins | Loss: 10 coins
                </div>
                <div className={`text-sm mt-2 ${battleStartDisabled ? 'text-gray-500' : 'text-blue-200'}`}>
                  🎫 Costs {tokenData?.token_cost_1v1 || 1} token{(tokenData?.token_cost_1v1 || 1) !== 1 ? 's' : ''}
                </div>
              </motion.button>

              {/* 5v5 Battle */}
              <motion.button
                whileHover={!battleStartDisabled ? { scale: 1.05, y: -5 } : {}}
                whileTap={!battleStartDisabled ? { scale: 0.95 } : {}}
                onClick={() => !battleStartDisabled && handleStartBattle('5v5')}
                disabled={battleStartDisabled}
                className={`rounded-xl p-6 shadow-lg transition-all ${
                  battleStartDisabled
                    ? 'bg-gray-700 text-gray-500 cursor-not-allowed opacity-50'
                    : 'bg-gradient-to-br from-purple-600 to-purple-700 hover:from-purple-500 hover:to-purple-600 text-white'
                }`}
              >
                <div className="text-5xl mb-4">🏆</div>
                <h2 className="text-2xl font-bold mb-2">5v5 Battle</h2>
                <p className={`text-sm mb-4 ${battleStartDisabled ? 'text-gray-500' : 'text-purple-100'}`}>
                  Epic battle with your full team
                </p>
                <div className={`font-semibold ${battleStartDisabled ? 'text-gray-500' : 'text-yellow-300'}`}>
                  💰 Win: 150 coins | Loss: 25 coins
                </div>
                <div className={`text-xs mt-2 ${battleStartDisabled ? 'text-gray-500' : 'text-green-300'}`}>
                  + Choose 1 opponent Pokemon on victory!
                </div>
                <div className={`text-sm mt-1 ${battleStartDisabled ? 'text-gray-500' : 'text-purple-200'}`}>
                  🎫 Costs {tokenData?.token_cost_5v5 || 2} token{(tokenData?.token_cost_5v5 || 2) !== 1 ? 's' : ''}
                </div>
              </motion.button>
            </div>

            <div className="mt-8 text-center">
              <button
                onClick={() => navigate('/dashboard')}
                className="text-gray-400 hover:text-white transition-colors"
              >
                ← Back to Dashboard
              </button>
            </div>
          </div>
        </motion.div>
      </div>
    );
  }

  // Loading screen
  if (loading && !battleState) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center">
        <div className="text-center">
          <motion.div
            animate={{ rotate: 360 }}
            transition={{ duration: 1, repeat: Infinity, ease: 'linear' }}
            className="text-6xl mb-4"
          >
            ⚔️
          </motion.div>
          <div className="text-white text-xl">Preparing battle...</div>
        </div>
      </div>
    );
  }

  // Battle screen
  return (
    <>
      <AnimatePresence>
        {showEntryAnimation && battleState && (
          <BattleEntryAnimation
            playerPokemon={battleState.player_deck?.[battleState.player_active_idx]}
            aiPokemon={battleState.ai_deck?.[battleState.ai_active_idx]}
            onComplete={handleEntryAnimationComplete}
          />
        )}
      </AnimatePresence>

      {!showEntryAnimation && (
        <BattleArena
          battleState={battleState}
          onMove={handleMove}
          onSwitchPokemon={handleSwitchPokemon}
          onSelectReward={handleSelectReward}
          onRematch={handleRematch}
          onNewBattle={handleNewBattle}
          onReturnToMenu={handleReturnToMenu}
          loading={loading}
          error={error}
        />
      )}
    </>
  );
}
