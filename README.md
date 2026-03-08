# lsm — Local Secrets Manager

A lightweight CLI for managing per-app, per-environment secrets encrypted with [age](https://github.com/FiloSottile/age). No remote services, no billing, no accounts.

## Install

```bash
go install github.com/llbbl/lsm@latest
```

## Quick Start

```bash
# 1. Generate encryption key
lsm init

# 2. Register your project
cd ~/Web/myapp
lsm link myapp

# 3. Import existing .env file
lsm import .env.local

# 4. Run with secrets injected
lsm exec -- pnpm dev

# 5. Export for deployment (writes to myapp.production.env)
lsm dump --env production
```

## How It Works

Secrets are stored as age-encrypted `.env` files in `~/.lsm/`:

```
~/.lsm/
  key.txt                      # age private key (chmod 600)
  config.yaml                  # default env + app registry
  myapp.dev.age                # encrypted secrets
  myapp.production.age
  backend.dev.age
```

The central `config.yaml` maps app names to project directories:

```yaml
env: dev
apps:
    myapp: /Users/you/Web/myapp
    backend: /Users/you/Web/backend
```

When you run any lsm command, it resolves which app you mean by checking your current directory against this registry.

## Commands

```bash
lsm init                              # Generate age key pair
lsm link <app>                        # Register current directory as an app
lsm set <KEY> <VALUE>                 # Set a secret (use '-' to read from stdin)
lsm get <KEY>                         # Get a secret value
lsm delete <KEY>                      # Remove a secret
lsm list                              # List secret keys (no values)
lsm dump                              # Export to .env file (masked terminal output)
lsm exec -- <command>                 # Inject secrets and run command
lsm edit                              # Edit secrets in $EDITOR
lsm import <file>                     # Import from .env file (or '-' for stdin)
lsm apps                              # List all registered apps
lsm envs <app>                        # List environments for an app
```

All commands accept `--app`, `--env`, and `--dir` flags to override auto-detection.

See [docs/commands.md](docs/commands.md) for detailed usage and examples.

## App & Env Resolution

lsm resolves the app name and environment in this order:

1. CLI flags (`--app`, `--env`)
2. `.lsm.yaml` in current directory (for backward compatibility)
3. Central registry lookup by current directory path
4. Directory name fallback, `~/.lsm/config.yaml` for default env

The recommended approach is to use `lsm link` to register your projects. After linking, lsm automatically knows which app you're working on based on your current directory:

```bash
cd ~/Web/myapp
lsm link myapp        # one-time setup

# Now all commands auto-resolve to app=myapp
lsm set DB_URL postgres://localhost
lsm exec -- pnpm dev
lsm dump --env production
```

## Security

- Age encryption (X25519 + ChaCha20-Poly1305)
- Private key stays in `~/.lsm/key.txt` (chmod 600)
- `exec` injects secrets into the subprocess only — not your shell
- `dump` masks values in terminal output, writes real values to file only
- Encrypted at rest — safe for screensharing

## Docs

- [Getting Started](docs/getting-started.md) — full setup walkthrough
- [Command Reference](docs/commands.md) — detailed usage for every command

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for release history. This project uses [Conventional Commits](https://www.conventionalcommits.org/) and [git-cliff](https://git-cliff.org/) for automated changelog generation.

## License

BSD 3-Clause. See [LICENSE](LICENSE).
