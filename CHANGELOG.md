# Changelog

All notable changes to PokeTacTix will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Pokemon evolution: leveling a Pokemon past its evolution threshold now
  evolves it into the next form (name, sprite, types, and base stats update).
  Evolution edges and level thresholds come from the PokéAPI evolution-chain
  data, stored in the new `evolution_chain.links` column. Only level-up
  evolutions apply; item/trade evolutions are ignored. Evolutions are shown
  on the battle result screen and counted in the `poketactix_evolutions_total`
  metric.

### Changed
- Nothing yet

### Fixed
- Nothing yet

## [1.0.0] - 2025-11-22

### Added
- Initial release of PokeTacTix CLI
- Complete offline Pokemon battle game
- 649 Pokemon from Generations 1-5
- Two battle modes: 1v1 and 5v5
- Pokemon collection and deck management
- Shop system with dynamic inventory
- Statistics and battle history tracking
- Local save system with automatic backups
- ASCII art graphics with ANSI color support
- Cross-platform binaries for Windows, macOS, and Linux
- Comprehensive documentation and installation scripts

### Features
- **Offline Gameplay**: No internet required, all data embedded
- **Battle System**: Strategic turn-based battles with type effectiveness
- **Progression**: Level up Pokemon through battles
- **Collection**: Collect Pokemon from battles and shop purchases
- **Customization**: Build and customize your 5-Pokemon deck
- **Statistics**: Track wins, losses, and progress
- **Quality of Life**: Auto-save, backups, battle speed settings

### Technical
- Built with Go for performance and portability
- Single binary with no external dependencies
- Embedded Pokemon data (~5MB)
- Local JSON save files with compression
- Terminal resize handling (Unix)
- Color detection and fallback support

---

## Version History

- **1.0.0** - Initial release

---

## How to Update

### Automatic (Recommended)
Run the installation script again:

**macOS/Linux:**
```bash
curl -L https://github.com/ifrunruhin12/poketactix/raw/main/scripts/install.sh | bash
```

**Windows:**
```powershell
powershell -ExecutionPolicy Bypass -Command "iwr https://github.com/ifrunruhin12/poketactix/raw/main/scripts/install.ps1 | iex"
```

### Manual
1. Download the latest binary from [releases](https://github.com/ifrunruhin12/poketactix/releases/latest)
2. Replace your existing binary
3. Your save file will be preserved

---

## Breaking Changes

None yet! We'll document any breaking changes here in future releases.

---

## Migration Guide

### From Pre-1.0 Versions
If you were using a development version, your save file should be compatible. If you encounter issues:

1. Backup your save file: `~/.poketactix/save.json`
2. Delete the save file
3. Restart the game
4. If needed, manually edit the backup to match the new format

---

## Support

- **Issues**: [GitHub Issues](https://github.com/ifrunruhin12/poketactix/issues)
- **Documentation**: [README.md](README.md)
