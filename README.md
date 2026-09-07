# lsm — local secrets without another service

`lsm` keeps each project's secrets encrypted on your machine with
[age](https://github.com/FiloSottile/age). Link a directory once, then run your
app with the right secrets for `dev`, `staging`, or `production`—without a
hosted vault, account, or subscription.

It works with the tools you already use: import an existing `.env`, inject
secrets directly into a process, export when a platform needs a file, or push
them to GitHub Actions.

## Install

```bash
brew install llbbl/tap/lsm
```

Prefer Go, a curl installer, or Windows? See the
[installation guide](docs/getting-started.md#1-install).

## From `.env` to encrypted in under a minute

```bash
# Create your local age key once
lsm init

# Register this project
cd ~/Web/myapp
lsm link myapp

# Encrypt the secrets you already have
lsm import .env.local

# Start the app with secrets injected into the child process
lsm exec -- pnpm dev
```

After importing, `lsm clean` can verify the encrypted copy before removing
plaintext `.env` files.

## Why lsm

- **Local-first:** encrypted files and the private key stay under `~/.lsm/`.
- **Project-aware:** commands resolve the linked app from your current directory.
- **Environment-aware:** keep separate encrypted stores for development,
  staging, production, or any environment you name.
- **Shell-friendly:** set, get, import, edit, list, and inject secrets without
  changing how your application reads environment variables.
- **Deployment-ready:** export a `.env` file when needed or sync directly to
  GitHub Actions secrets.
- **Observable when you want it:** optional OTLP audit events are off by default.

## A few everyday commands

```bash
lsm set API_KEY                    # prompt for a value, hidden, never in shell history
lsm edit                           # edit the current encrypted secret set
lsm list                           # list keys without revealing values
lsm exec -- go run ./cmd/server    # inject secrets into a process
lsm dump --env production          # export a specific environment
lsm gh push                        # sync to GitHub Actions secrets
```

Run `lsm <command> --help` for flags and examples.

## Documentation

- [Getting Started](docs/getting-started.md) — installation and a complete first workflow
- [Command Reference](docs/commands.md) — every command, flag, and resolution rule
- [Observability](docs/observability.md) — audit events, redaction, and OTLP configuration
- [Development](docs/development.md) — contributing, testing, and releases

Release history is in the [changelog](CHANGELOG.md).

## License

BSD 3-Clause. See [LICENSE](LICENSE).
