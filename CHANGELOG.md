# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Features

- Implement lsm CLI for local secrets management
- **config:** Add central app registry in config.yaml
- **cmd:** Mask dump output and write secrets to file

### Refactoring

- **exec:** Use cmd.ArgsLenAtDash() instead of scanning os.Args
- **cmd:** Extract helpers and split test files

### Testing

- **cmd:** Expand init command tests into dedicated file

