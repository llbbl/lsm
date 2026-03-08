# Command Reference

All commands accept these global flags:

| Flag | Description |
|------|-------------|
| `--app`, `-a` | App name (overrides auto-detection) |
| `--env`, `-e` | Environment name (overrides config default) |
| `--dir`, `-d` | Path to lsm directory (default: `~/.lsm`) |

Most commands also accept positional `[app] [env]` arguments before their required args, so you can write `lsm get myapp production DB_URL` instead of `lsm get --app myapp --env production DB_URL`.

---

## init

Generate a new age encryption key pair.

```bash
lsm init
lsm init --force    # overwrite existing key
```

Creates `~/.lsm/` with `key.txt` and `config.yaml` (if they don't exist). The `config.yaml` starts with `env: dev` as the default environment.

Use `--force` to regenerate the key. This will make all previously encrypted secrets unreadable.

---

## link

Register the current directory as an app in the central config.

```bash
cd ~/Web/myapp
lsm link myapp
```

This writes an entry to `~/.lsm/config.yaml`:

```yaml
apps:
    myapp: /Users/you/Web/myapp
```

After linking, any lsm command run from that directory automatically resolves to the linked app name. If you re-link the same directory with a different name, the old mapping is removed.

---

## set

Set or update a secret.

```bash
lsm set KEY VALUE
lsm set DB_URL postgres://localhost

# Read value from stdin (avoids shell history)
echo -n "secret" | lsm set API_KEY -
```

If the key already exists, its value is updated.

---

## get

Get a single secret value.

```bash
lsm get KEY
lsm get DB_URL
```

Outputs just the raw value with no trailing newline, making it suitable for command substitution:

```bash
psql "$(lsm get DB_URL)"
```

Returns an error if the key doesn't exist.

---

## delete

Remove a secret.

```bash
lsm delete KEY
```

Returns an error if the key doesn't exist.

---

## list

List all secret key names (without values).

```bash
lsm list
```

Output:

```
DB_URL
API_KEY
STRIPE_SECRET
```

---

## dump

Export secrets to a `.env` file. Terminal output shows masked values for safety.

```bash
lsm dump
lsm dump --output .env.deploy
lsm dump --env production
```

**Default behavior:**
- Writes real `KEY=VALUE` content to `{app}.{env}.env` in the current directory
- Prints masked output to the terminal (e.g., `API_KEY=sk********`)
- Shows a confirmation message with the file path

| Flag | Description |
|------|-------------|
| `--output`, `-o` | Custom output file path |

**Masking rules:** Short values are fully masked. Longer values show the first 1-2 characters. Very long values are capped at 10 characters of masked output.

---

## exec

Inject secrets as environment variables and run a command.

```bash
lsm exec -- pnpm dev
lsm exec -- go run ./cmd/server
lsm exec -- docker compose up
lsm exec --env production -- node server.js
```

The `--` separates lsm flags from the command to run. Secrets are injected into the subprocess environment only. If a secret key matches an existing environment variable, the secret value takes precedence.

---

## edit

Open decrypted secrets in your editor, re-encrypt on save.

```bash
lsm edit
```

Uses `$EDITOR`, then `$VISUAL`, then falls back to `vi`. The decrypted content is written to a temp file, which is securely overwritten with zeros and deleted after the editor exits.

---

## import

Bulk import key-value pairs from a `.env` file.

```bash
lsm import .env.local
lsm import /path/to/secrets.env
cat .env | lsm import -
```

Merges imported keys into the existing store. If a key already exists, the imported value overwrites it. Comments and blank lines in the source file are preserved in the encrypted store.

Supports:
- Unquoted values: `KEY=value`
- Single-quoted values: `KEY='value'`
- Double-quoted values (including multiline): `KEY="line1\nline2"`
- `export` prefix: `export KEY=value`
- Comments: `# comment`

---

## apps

List all app namespaces that have encrypted secret files.

```bash
lsm apps
```

Scans `~/.lsm/` for `.age` files and extracts unique app names.

---

## envs

List all environments for a given app.

```bash
lsm envs myapp
```

Output:

```
dev
production
staging
```
