# lsm — Local Secrets Manager

A lightweight CLI for managing per-app, per-environment secrets encrypted with [age](https://github.com/FiloSottile/age). No remote services, no billing, no accounts.

## Why

| Tool | Problem |
|------|---------|
| Doppler | 10 project free limit, $21/mo after |
| Infisical | 3 project free limit, $18/identity/mo |
| GCP Secret Manager | Flat namespace, can't reuse env var names across apps |
| `.env` files | Plaintext on disk, leak on screenshare or git |

lsm encrypts secrets locally with age. Each app gets its own encrypted file. Secrets are only decrypted into subprocesses or stdout.

## Install

```bash
go install github.com/llbbl/lsm@latest
```

## Quick Start

```bash
# Generate encryption key
lsm init

# Import existing .env file
cd ~/Web/myapp
lsm import .env.local

# Run with secrets injected
lsm exec -- pnpm dev

# Dump for pasting into Coolify/Railway
lsm dump --env production
```

## How It Works

Secrets are stored as age-encrypted `.env` files in `~/.lsm/`:

```
~/.lsm/
  key.txt                      # age private key
  config.yaml                  # default env (e.g., dev)
  myapp.dev.age                # encrypted secrets
  myapp.production.age
  backend.dev.age
```

App name is inferred from your current directory. Environment defaults to `dev` (configurable).

## Commands

```bash
lsm init                              # Generate age key pair
lsm set <KEY> <value>                  # Set a secret
lsm get <KEY>                          # Get a secret value
lsm delete <KEY>                       # Remove a secret
lsm list                               # List secret keys (no values)
lsm dump                               # Output KEY=VALUE format
lsm exec -- <command>                  # Inject secrets and run command
lsm edit                               # Edit secrets in $EDITOR
lsm import <file>                      # Import from .env file
lsm apps                               # List all apps
lsm envs <app>                         # List environments for an app
lsm link <app> [env]                   # Link directory to app name
```

All commands accept `--app` and `--env` flags to override auto-detection.

## App & Env Resolution

1. CLI flags (`--app`, `--env`)
2. `.lsm.yaml` in current directory
3. Directory name = app name, `~/.lsm/config.yaml` = default env

```bash
# In ~/Web/positivehelp with default env: dev
lsm exec -- pnpm dev              # app=positivehelp, env=dev
lsm dump --env production         # app=positivehelp, env=production
```

## Security

- Age encryption (X25519 + ChaCha20-Poly1305)
- Private key stays in `~/.lsm/key.txt` (chmod 600)
- `exec` injects into subprocess only — not your shell
- Encrypted at rest — safe for screensharing

## License

BSD 3-Clause. See [LICENSE](LICENSE).
