# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0] - 2026-03-09

### Bug Fixes

- Resolve all errcheck lint violations

### Documentation

- Update README and add command reference

### Features

- Implement lsm CLI for local secrets management
- **config:** Add central app registry in config.yaml
- **cmd:** Mask dump output and write secrets to file
- **cmd:** Auto-detect .env files in import command
- **cmd:** Dump outputs .env with overwrite prompt and gitignore safety
- **cmd:** Add clean command to remove .env files after verification

### Refactoring

- **exec:** Use cmd.ArgsLenAtDash() instead of scanning os.Args
- **cmd:** Extract helpers and split test files

### Testing

- **cmd:** Expand init command tests into dedicated file

