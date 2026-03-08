# Getting Started with lsm

This guide walks through the full setup: from installing lsm to running your app with encrypted secrets.

## 1. Install

```bash
go install github.com/llbbl/lsm@latest
```

Verify the installation:

```bash
lsm --help
```

## 2. Initialize

Generate an age encryption key pair. This only needs to happen once:

```bash
lsm init
```

This creates `~/.lsm/` with:
- `key.txt` — your private key (chmod 600, never share this)
- `config.yaml` — global configuration (default environment, app registry)

The output shows your public key (`age1...`). You don't need it for local use, but it's useful if you later want to encrypt secrets for this machine from elsewhere.

## 3. Register Your Project

Navigate to your project directory and register it:

```bash
cd ~/Web/myapp
lsm link myapp
# Output: Linked myapp -> /Users/you/Web/myapp
```

This adds an entry to `~/.lsm/config.yaml` mapping the name "myapp" to your project's absolute path. From now on, whenever you're in that directory, lsm knows you're working with "myapp" — no flags needed.

You can link as many projects as you want:

```bash
cd ~/Web/backend
lsm link backend

cd ~/Web/positive-tui
lsm link positive-tui
```

To see all registered apps:

```bash
lsm apps
```

## 4. Add Secrets

There are three ways to get secrets into lsm:

### Import from an existing .env file

If you already have a `.env` or `.env.local` file:

```bash
cd ~/Web/myapp
lsm import .env.local
```

This reads all `KEY=VALUE` pairs from the file and encrypts them. The original file is untouched — you can delete it afterward if you want.

You can also pipe from stdin:

```bash
cat .env.local | lsm import -
```

### Set individual secrets

```bash
lsm set DB_URL postgres://localhost:5432/mydb
lsm set API_KEY sk-1234567890
```

For sensitive values, read from stdin to avoid shell history:

```bash
echo -n "super-secret" | lsm set API_KEY -
```

### Edit all secrets at once

Open the decrypted secrets in your editor:

```bash
lsm edit
```

This decrypts to a temp file, opens `$EDITOR` (or `$VISUAL`, or `vi`), and re-encrypts when you save and quit. The temp file is securely wiped after.

## 5. Use Secrets

### Run a command with secrets injected

```bash
lsm exec -- pnpm dev
lsm exec -- go run ./cmd/server
lsm exec -- docker compose up
```

The `--` separates lsm's arguments from the command. Secrets are injected as environment variables into the subprocess only — they don't leak into your shell.

### Read a single secret

```bash
lsm get DB_URL
```

Outputs just the value, useful for scripting:

```bash
psql "$(lsm get DB_URL)"
```

### List keys (without values)

```bash
lsm list
```

### Export to a .env file

```bash
lsm dump
```

This writes the real secrets to `.env` and shows masked values in the terminal. If `.env` already exists, you'll be prompted before overwriting. If you're in a git repo, lsm automatically adds `.env` to `.gitignore`.

Use `--output` to customize the filename:

```bash
lsm dump --output .env.deploy
```

This is useful for pasting into deployment platforms like Coolify, Railway, or Vercel.

## 6. Working with Multiple Environments

By default, lsm uses the `dev` environment (set in `~/.lsm/config.yaml`). Override with `--env`:

```bash
# Set a production secret
lsm set --env production DB_URL postgres://prod-host:5432/mydb

# Run with production secrets
lsm exec --env production -- pnpm start

# Export production secrets
lsm dump --env production

# List all environments for an app
lsm envs myapp
```

Each environment is stored as a separate encrypted file: `myapp.dev.age`, `myapp.production.age`, etc.

## Typical Workflow

Here's what daily usage looks like:

```bash
# Morning: start working on myapp
cd ~/Web/myapp
lsm exec -- pnpm dev

# Need to add a new secret
lsm set STRIPE_KEY sk_test_abc123

# Quick edit of multiple secrets
lsm edit

# Deploy: export production secrets
lsm dump --env production
# Copy the contents of myapp.production.env to your hosting provider

# Start a new project
cd ~/Web/newproject
lsm link newproject
lsm import .env.example
lsm set SECRET_KEY $(openssl rand -hex 32)
```

## Next Steps

- See [commands.md](commands.md) for the full command reference
- Run `lsm <command> --help` for any command's flags and usage
