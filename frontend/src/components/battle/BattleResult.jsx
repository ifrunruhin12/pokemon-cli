import PropTypes from 'prop-types';
import { motion, AnimatePresence } from 'framer-motion';
import { useState } from 'react';
import PokemonCard from './PokemonCard';

/**
 * BattleResult Component
 * Displays victory/defeat/draw message with rewards
 * Shows coins earned, XP gained, level-up notifications
 * For 5v5 victories, shows AI Pokemon selection screen
 */
const BattleResult = ({ 
  result = 'victory', // 'victory', 'defeat', 'draw'
  rewards = {},
  aiPokemon = [],
  onSelectReward,
  onRematch,
  onNewBattle,
  onReturnToMenu,
  className = ''
}) => {
  const [selectedReward, setSelectedReward] = useState(null);
  const [rewardClaimed, setRewardClaimed] = useState(false);
  const { 
    coins_earned = 0, 
    xp_gained = {}, 
    level_ups = [],
    evolutions = [], // Array of pokemon that evolved after the battle
    pokemon_details = [], // Array of pokemon with their XP and level info
    newly_unlocked_achievements = [] // Array of newly unlocked achievements
  } = rewards;

  // Get result styling
  const getResultStyle = () => {
    switch (result) {
      case 'victory':
        return {
          bg: 'from-green-600 to-emerald-700',
          text: 'Victory!',
          icon: '🏆',
          message: 'You won the battle!'
        };
      case 'defeat':
        return {
          bg: 'from-red-600 to-rose-700',
          text: 'Defeat',
          icon: '💔',
          message: 'You lost the battle...'
        };
      case 'draw':
        return {
          bg: 'from-gray-600 to-gray-700',
          text: 'Draw',
          icon: '🤝',
          message: 'The battle ended in a draw'
        };
      default:
        return {
          bg: 'from-gray-600 to-gray-700',
          text: 'Battle Over',
          icon: '⚔️',
          message: 'The battle has ended'
        };
    }
  };

  const style = getResultStyle();
  const showRewardSelection = result === 'victory' && aiPokemon && aiPokemon.length > 0 && !rewardClaimed;

  // Handle reward selection
  const handleRewardSelect = async (index) => {
    setSelectedReward(index);
    if (onSelectReward) {
      try {
        await onSelectReward(index);
        setRewardClaimed(true);
      } catch (error) {
        console.error('Failed to claim reward:', error);
        // Reset selection on error
        setSelectedReward(null);
      }
    }
  };

  return (
    <AnimatePresence>
      <motion.div
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        className={`fixed inset-0 bg-black/80 backdrop-blur-sm flex items-center justify-center z-50 p-4 ${className}`}
      >
        <motion.div
          initial={{ opacity: 0, scale: 0.8, y: 50 }}
          animate={{ opacity: 1, scale: 1, y: 0 }}
          exit={{ opacity: 0, scale: 0.8, y: 50 }}
          transition={{ type: 'spring', stiffness: 200, damping: 20 }}
          className="bg-gray-800 rounded-2xl shadow-2xl max-w-4xl w-full max-h-[90vh] overflow-y-auto border-2 border-gray-700"
        >
          {/* Header with animated result */}
          <div className={`bg-gradient-to-r ${style.bg} p-6 text-center relative overflow-hidden`}>
            {/* Animated background particles */}
            {result === 'victory' && (
              <>
                {[...Array(20)].map((_, i) => (
                  <motion.div
                    key={i}
                    initial={{ opacity: 0, y: 0, x: Math.random() * 100 - 50 }}
                    animate={{ 
                      opacity: [0, 1, 0],
                      y: -100,
                      x: Math.random() * 100 - 50
                    }}
                    transition={{ 
                      duration: 2,
                      delay: i * 0.1,
                      repeat: Infinity,
                      repeatDelay: 1
                    }}
                    className="absolute text-2xl"
                    style={{ left: `${Math.random() * 100}%`, top: '100%' }}
                  >
                    ✨
                  </motion.div>
                ))}
              </>
            )}
            
            <motion.div
              initial={{ scale: 0, rotate: -180 }}
              animate={{ scale: 1, rotate: 0 }}
              transition={{ 
                delay: 0.2, 
                type: 'spring', 
                stiffness: 200,
                damping: 15
              }}
              className="text-7xl mb-4 relative z-10"
            >
              {style.icon}
            </motion.div>
            <motion.h2 
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: 0.4 }}
              className="text-4xl font-bold text-white mb-2 relative z-10"
            >
              {style.text}
            </motion.h2>
            <motion.p 
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: 0.5 }}
              className="text-xl text-white/90 relative z-10"
            >
              {style.message}
            </motion.p>
          </div>

          {/* Rewards section */}
          <div className="p-6 space-y-6">
            {/* Coins earned with animation */}
            <motion.div
              initial={{ opacity: 0, scale: 0.8 }}
              animate={{ opacity: 1, scale: 1 }}
              transition={{ delay: 0.6, type: 'spring', stiffness: 200 }}
              className="bg-gradient-to-r from-yellow-600 to-amber-600 rounded-lg p-6 text-center shadow-lg"
            >
              <motion.div 
                animate={{ 
                  rotate: [0, 10, -10, 10, 0],
                  scale: [1, 1.1, 1]
                }}
                transition={{ 
                  duration: 0.5,
                  delay: 0.8,
                  repeat: 2
                }}
                className="text-5xl mb-3"
              >
                💰
              </motion.div>
              <div className="text-white text-lg font-semibold">
                Coins Earned
              </div>
              <motion.div 
                initial={{ scale: 0 }}
                animate={{ scale: 1 }}
                transition={{ delay: 0.9, type: 'spring', stiffness: 300 }}
                className="text-yellow-200 text-4xl font-bold mt-2"
              >
                +{coins_earned}
              </motion.div>
            </motion.div>

            {/* XP gained per Pokemon with detailed breakdown */}
            {pokemon_details && pokemon_details.length > 0 && (
              <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.7 }}
                className="bg-gray-700 rounded-lg p-6 shadow-lg"
              >
                <h3 className="text-2xl font-bold text-white mb-4 flex items-center gap-2">
                  <span>⭐</span>
                  <span>Experience Gained</span>
                </h3>
                <div className="space-y-3">
                  {pokemon_details.map((pokemon, index) => (
                    <motion.div
                      key={index}
                      initial={{ opacity: 0, x: -20 }}
                      animate={{ opacity: 1, x: 0 }}
                      transition={{ delay: 0.8 + index * 0.1 }}
                      className="bg-gray-800 rounded-lg p-4 flex items-center justify-between"
                    >
                      <div className="flex items-center gap-3">
                        {pokemon.sprite ? (
                          <img src={pokemon.sprite} alt={pokemon.name} className="w-10 h-10 object-contain" />
                        ) : (
                          <div className="text-2xl">🎴</div>
                        )}
                        <div>
                          <div className="text-white font-semibold">{pokemon.name}</div>
                          <div className="text-sm text-gray-400">Level {pokemon.level}</div>
                        </div>
                      </div>
                      <div className="text-right">
                        <motion.div 
                          initial={{ scale: 0 }}
                          animate={{ scale: 1 }}
                          transition={{ delay: 0.9 + index * 0.1, type: 'spring' }}
                          className="text-purple-400 font-bold text-lg"
                        >
                          +{pokemon.xp_gained || 0} XP
                        </motion.div>
                        {pokemon.leveled_up && (
                          <motion.div
                            initial={{ opacity: 0, scale: 0 }}
                            animate={{ opacity: 1, scale: 1 }}
                            transition={{ delay: 1 + index * 0.1 }}
                            className="text-yellow-400 text-sm font-semibold flex items-center gap-1"
                          >
                            <span>🎉</span>
                            <span>Level Up!</span>
                          </motion.div>
                        )}
                      </div>
                    </motion.div>
                  ))}
                </div>
              </motion.div>
            )}

            {/* Legacy XP display (fallback) */}
            {(!pokemon_details || pokemon_details.length === 0) && Object.keys(xp_gained).length > 0 && (
              <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.7 }}
                className="bg-gray-700 rounded-lg p-4"
              >
                <h3 className="text-xl font-bold text-white mb-3 flex items-center gap-2">
                  <span>⭐</span>
                  <span>Experience Gained</span>
                </h3>
                <div className="grid grid-cols-2 gap-2">
                  {Object.entries(xp_gained).map(([cardId, xp], index) => (
                    <motion.div 
                      key={cardId}
                      initial={{ opacity: 0, scale: 0.8 }}
                      animate={{ opacity: 1, scale: 1 }}
                      transition={{ delay: 0.8 + index * 0.1 }}
                      className="bg-gray-800 rounded p-2 text-center"
                    >
                      <div className="text-purple-400 font-semibold">+{xp} XP</div>
                    </motion.div>
                  ))}
                </div>
              </motion.div>
            )}

            {/* Evolutions — the star of the show */}
            {evolutions && evolutions.length > 0 && (
              <motion.div
                initial={{ opacity: 0, scale: 0.8 }}
                animate={{ opacity: 1, scale: 1 }}
                transition={{ delay: 0.6, type: 'spring' }}
                className="bg-gradient-to-r from-indigo-600 via-purple-600 to-pink-600 rounded-lg p-6 shadow-xl border-2 border-yellow-400/50"
              >
                <h3 className="text-2xl font-bold text-white mb-4 flex items-center gap-2">
                  <motion.span
                    animate={{ rotate: [0, 15, -15, 0], scale: [1, 1.2, 1] }}
                    transition={{ repeat: Infinity, duration: 2 }}
                  >
                    ✨
                  </motion.span>
                  <span>Evolution!</span>
                </h3>
                <div className="space-y-4">
                  {evolutions.map((evo, index) => (
                    <motion.div
                      key={index}
                      initial={{ opacity: 0, y: 20 }}
                      animate={{ opacity: 1, y: 0 }}
                      transition={{ delay: 0.7 + index * 0.15 }}
                      className="bg-black/30 rounded-lg p-4 flex items-center justify-center gap-4"
                    >
                      {evo.sprite && (
                        <motion.img
                          src={evo.sprite}
                          alt={evo.into}
                          className="w-20 h-20 object-contain"
                          initial={{ scale: 0, rotate: -180 }}
                          animate={{ scale: 1, rotate: 0 }}
                          transition={{ delay: 0.8 + index * 0.15, type: 'spring', stiffness: 200 }}
                        />
                      )}
                      <div className="text-center">
                        <div className="text-lg text-gray-300 line-through">
                          {evo.from}
                        </div>
                        <motion.div
                          animate={{ y: [0, -3, 0] }}
                          transition={{ repeat: Infinity, duration: 1.5 }}
                          className="text-2xl font-bold text-white"
                        >
                          {evo.into}
                        </motion.div>
                        <div className="text-sm text-yellow-300">
                          Evolved at level {evo.level}
                        </div>
                      </div>
                    </motion.div>
                  ))}
                </div>
              </motion.div>
            )}

            {/* Level ups with stat increases */}
            {level_ups && level_ups.length > 0 && (
              <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.8 }}
                className="bg-gradient-to-r from-purple-600 to-pink-600 rounded-lg p-6 shadow-lg"
              >
                <motion.div
                  animate={{ 
                    scale: [1, 1.1, 1],
                    rotate: [0, 5, -5, 0]
                  }}
                  transition={{ 
                    duration: 0.5,
                    delay: 0.9,
                    repeat: 3
                  }}
                  className="text-center"
                >
                  <h3 className="text-2xl font-bold text-white mb-3 flex items-center justify-center gap-2">
                    <span className="text-4xl">🎉</span>
                    <span>Level Up!</span>
                  </h3>
                </motion.div>
                <div className="space-y-3 mt-4">
                  {level_ups.map((levelUp, index) => (
                    <motion.div
                      key={index}
                      initial={{ opacity: 0, x: -20 }}
                      animate={{ opacity: 1, x: 0 }}
                      transition={{ delay: 1 + index * 0.1 }}
                      className="bg-white/10 rounded-lg p-4"
                    >
                      <div className="text-white font-bold text-lg mb-2">
                        {levelUp.name} reached Level {levelUp.new_level}!
                      </div>
                      {levelUp.stat_increases && (
                        <div className="grid grid-cols-2 gap-2 text-sm">
                          {levelUp.stat_increases.hp && (
                            <div className="text-green-300">
                              ❤️ HP: +{levelUp.stat_increases.hp}
                            </div>
                          )}
                          {levelUp.stat_increases.attack && (
                            <div className="text-red-300">
                              ⚔️ Attack: +{levelUp.stat_increases.attack}
                            </div>
                          )}
                          {levelUp.stat_increases.defense && (
                            <div className="text-blue-300">
                              🛡️ Defense: +{levelUp.stat_increases.defense}
                            </div>
                          )}
                          {levelUp.stat_increases.speed && (
                            <div className="text-yellow-300">
                              ⚡ Speed: +{levelUp.stat_increases.speed}
                            </div>
                          )}
                        </div>
                      )}
                    </motion.div>
                  ))}
                </div>
              </motion.div>
            )}

            {/* Newly Unlocked Achievements */}
            {newly_unlocked_achievements && newly_unlocked_achievements.length > 0 && (
              <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.9 }}
                className="bg-gradient-to-r from-yellow-600 to-orange-600 rounded-lg p-6 shadow-lg border-2 border-yellow-400"
              >
                <motion.div
                  animate={{ 
                    scale: [1, 1.15, 1],
                    rotate: [0, -5, 5, 0]
                  }}
                  transition={{ 
                    duration: 0.6,
                    delay: 1,
                    repeat: 2
                  }}
                  className="text-center mb-4"
                >
                  <h3 className="text-3xl font-bold text-white flex items-center justify-center gap-3">
                    <span className="text-5xl">🏆</span>
                    <span>Achievement{newly_unlocked_achievements.length > 1 ? 's' : ''} Unlocked!</span>
                  </h3>
                </motion.div>
                <div className="space-y-3">
                  {newly_unlocked_achievements.map((achievement, index) => (
                    <motion.div
                      key={achievement.id}
                      initial={{ opacity: 0, scale: 0.8, rotateY: 90 }}
                      animate={{ opacity: 1, scale: 1, rotateY: 0 }}
                      transition={{ 
                        delay: 1.1 + index * 0.15,
                        type: 'spring',
                        stiffness: 200
                      }}
                      className="bg-white/20 backdrop-blur-sm rounded-lg p-4 border border-yellow-300/50"
                    >
                      <div className="flex items-center gap-4">
                        <motion.div
                          animate={{ 
                            rotate: [0, 360],
                            scale: [1, 1.2, 1]
                          }}
                          transition={{ 
                            duration: 0.8,
                            delay: 1.2 + index * 0.15
                          }}
                          className="text-5xl"
                        >
                          {achievement.icon || '🏆'}
                        </motion.div>
                        <div className="flex-1">
                          <div className="text-white font-bold text-xl mb-1">
                            {achievement.name}
                          </div>
                          <div className="text-yellow-100 text-sm">
                            {achievement.description}
                          </div>
                        </div>
                        <motion.div
                          initial={{ scale: 0 }}
                          animate={{ scale: 1 }}
                          transition={{ 
                            delay: 1.3 + index * 0.15,
                            type: 'spring',
                            stiffness: 300
                          }}
                          className="text-4xl"
                        >
                          ✨
                        </motion.div>
                      </div>
                    </motion.div>
                  ))}
                </div>
                {/* Celebration particles */}
                {[...Array(15)].map((_, i) => (
                  <motion.div
                    key={i}
                    initial={{ opacity: 0, y: 0, x: 0, scale: 0 }}
                    animate={{ 
                      opacity: [0, 1, 1, 0],
                      y: [-50, -150],
                      x: [(Math.random() - 0.5) * 100, (Math.random() - 0.5) * 200],
                      scale: [0, 1, 1, 0],
                      rotate: [0, 360]
                    }}
                    transition={{ 
                      duration: 2,
                      delay: 1 + i * 0.1,
                      repeat: Infinity,
                      repeatDelay: 2
                    }}
                    className="absolute text-2xl pointer-events-none"
                    style={{ 
                      left: `${20 + Math.random() * 60}%`, 
                      top: '50%'
                    }}
                  >
                    {['🎉', '⭐', '✨', '🏆', '💫'][i % 5]}
                  </motion.div>
                ))}
              </motion.div>
            )}

            {/* AI Pokemon selection for 5v5 victories */}
            {showRewardSelection && (
              <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.9 }}
                className="bg-gradient-to-br from-gray-700 to-gray-800 rounded-lg p-6 shadow-lg border-2 border-yellow-500"
              >
                <motion.div
                  initial={{ scale: 0 }}
                  animate={{ scale: 1 }}
                  transition={{ delay: 1, type: 'spring', stiffness: 200 }}
                >
                  <h3 className="text-3xl font-bold text-white mb-2 text-center flex items-center justify-center gap-2">
                    <span className="text-4xl">🎁</span>
                    <span>Choose Your Reward Pokémon!</span>
                  </h3>
                </motion.div>
                <motion.p 
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  transition={{ delay: 1.1 }}
                  className="text-gray-300 text-center mb-6 text-lg"
                >
                  Select one Pokémon from your opponent's team to add to your collection
                </motion.p>
                <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-4 mb-6">
                  {aiPokemon.map((pokemon, index) => (
                    <motion.div
                      key={index}
                      initial={{ opacity: 0, scale: 0.5, rotateY: 180 }}
                      animate={{ opacity: 1, scale: 1, rotateY: 0 }}
                      transition={{ 
                        delay: 1.2 + index * 0.1,
                        type: 'spring',
                        stiffness: 200
                      }}
                      whileHover={{ scale: 1.1, y: -10, rotateZ: 5 }}
                      whileTap={{ scale: 0.95 }}
                      onClick={() => setSelectedReward(index)}
                      className={`cursor-pointer transition-all ${
                        selectedReward === index 
                          ? 'ring-4 ring-yellow-400 rounded-lg' 
                          : ''
                      }`}
                    >
                      <PokemonCard
                        pokemon={pokemon}
                        isActive={false}
                        isFaceDown={false}
                        isKnockedOut={false}
                      />
                      {selectedReward === index && (
                        <motion.div
                          initial={{ scale: 0 }}
                          animate={{ scale: 1 }}
                          className="text-center mt-2 text-yellow-400 font-bold"
                        >
                          ✓ Selected
                        </motion.div>
                      )}
                    </motion.div>
                  ))}
                </div>
                
                {/* Claim Reward Button */}
                {selectedReward !== null && (
                  <motion.div
                    initial={{ opacity: 0, y: 20 }}
                    animate={{ opacity: 1, y: 0 }}
                    className="flex justify-center"
                  >
                    <motion.button
                      whileHover={{ scale: 1.05, y: -2 }}
                      whileTap={{ scale: 0.95 }}
                      onClick={() => handleRewardSelect(selectedReward)}
                      className="px-8 py-4 bg-gradient-to-r from-yellow-500 to-orange-500 hover:from-yellow-400 hover:to-orange-400 text-white font-bold rounded-xl shadow-lg transition-all flex items-center gap-2 text-lg"
                    >
                      <span className="text-2xl">🎁</span>
                      <span>Claim {aiPokemon[selectedReward]?.name || 'Pokémon'}</span>
                    </motion.button>
                  </motion.div>
                )}
              </motion.div>
            )}

            {/* Action buttons */}
            {!showRewardSelection && (
              <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 1 }}
                className="flex flex-wrap gap-4 justify-center pt-4"
              >
                {onRematch && (
                  <motion.button
                    whileHover={{ scale: 1.05, y: -2 }}
                    whileTap={{ scale: 0.95 }}
                    onClick={onRematch}
                    className="px-8 py-4 bg-gradient-to-r from-blue-600 to-blue-700 hover:from-blue-500 hover:to-blue-600 text-white font-bold rounded-xl shadow-lg transition-all flex items-center gap-2 text-lg"
                  >
                    <span className="text-2xl">🔄</span>
                    <span>Rematch</span>
                  </motion.button>
                )}
                {onNewBattle && (
                  <motion.button
                    whileHover={{ scale: 1.05, y: -2 }}
                    whileTap={{ scale: 0.95 }}
                    onClick={onNewBattle}
                    className="px-8 py-4 bg-gradient-to-r from-green-600 to-green-700 hover:from-green-500 hover:to-green-600 text-white font-bold rounded-xl shadow-lg transition-all flex items-center gap-2 text-lg"
                  >
                    <span className="text-2xl">⚔️</span>
                    <span>New Battle</span>
                  </motion.button>
                )}
                {onReturnToMenu && (
                  <motion.button
                    whileHover={{ scale: 1.05, y: -2 }}
                    whileTap={{ scale: 0.95 }}
                    onClick={onReturnToMenu}
                    className="px-8 py-4 bg-gradient-to-r from-gray-600 to-gray-700 hover:from-gray-500 hover:to-gray-600 text-white font-bold rounded-xl shadow-lg transition-all flex items-center gap-2 text-lg"
                  >
                    <span className="text-2xl">🏠</span>
                    <span>Return to Dashboard</span>
                  </motion.button>
                )}
              </motion.div>
            )}
          </div>
        </motion.div>
      </motion.div>
    </AnimatePresence>
  );
};

BattleResult.propTypes = {
  result: PropTypes.oneOf(['victory', 'defeat', 'draw']),
  rewards: PropTypes.shape({
    coins_earned: PropTypes.number,
    xp_gained: PropTypes.object,
    level_ups: PropTypes.arrayOf(PropTypes.shape({
      name: PropTypes.string,
      new_level: PropTypes.number,
      stat_increases: PropTypes.shape({
        hp: PropTypes.number,
        attack: PropTypes.number,
        defense: PropTypes.number,
        speed: PropTypes.number
      })
    })),
    pokemon_details: PropTypes.arrayOf(PropTypes.shape({
      name: PropTypes.string,
      level: PropTypes.number,
      xp_gained: PropTypes.number,
      leveled_up: PropTypes.bool,
      evolved: PropTypes.bool,
      sprite: PropTypes.string
    })),
    evolutions: PropTypes.arrayOf(PropTypes.shape({
      from: PropTypes.string,
      into: PropTypes.string,
      level: PropTypes.number,
      sprite: PropTypes.string
    })),
    newly_unlocked_achievements: PropTypes.arrayOf(PropTypes.shape({
      id: PropTypes.number,
      name: PropTypes.string,
      description: PropTypes.string,
      icon: PropTypes.string,
      requirement_type: PropTypes.string,
      requirement_value: PropTypes.number,
      unlocked: PropTypes.bool,
      unlocked_at: PropTypes.string
    }))
  }),
  aiPokemon: PropTypes.arrayOf(PropTypes.object),
  onSelectReward: PropTypes.func,
  onRematch: PropTypes.func,
  onNewBattle: PropTypes.func,
  onReturnToMenu: PropTypes.func,
  className: PropTypes.string
};

export default BattleResult;
