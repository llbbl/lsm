# Command Reference

Most commands accept these global flags:

| Flag | Description |
|------|-------------|
| `--app`, `-a` | App name (overrides auto-detection) |
| `--env`, `-e` | Environment name (overrides config default) |
| `--dir`, `-d` | Path to lsm directory (default: `~/.lsm`) |

Most commands also accept positional `[app] [env]` arguments before their required args, so you can write `lsm get myapp production DB_URL` instead of `lsm get --app myapp --env production DB_URL`.

`lsm gh` is stricter: it rejects `--app` and only uses the app registered for the current directory with `lsm link`.

## App and Environment Resolution

For non-`gh` commands, app resolution is deliberate:

1. `--app` or positional `[app]`
2. `.lsm.yaml` in the current directory
3. `~/.lsm/config.yaml` registry lookup for the current directory path

If none of those identifies an app, lsm errors and tells you to run `lsm link <app>` in the project, pass `--app`, or create `.lsm.yaml`. It does not guess the app from the current directory name.

Environment resolution is:

1. `--env` or positional `[env]`
2. `.lsm.yaml` in the current directory
3. The default `env` in `~/.lsm/config.yaml`

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

### Providing the value

There are three ways to supply the value:

| Form | Value source |
| --- | --- |
| `lsm set KEY VALUE` | the command line (stored verbatim) |
| `lsm set KEY -` | stdin |
| `lsm set KEY` | interactive prompt, or piped stdin |

Positional app/env always precede a **required** KEY VALUE, so they are never
confused with the value:

```bash
lsm set app KEY VALUE
lsm set app env KEY VALUE
```

The value is optional **only** when KEY is the single positional argument. To
prompt (or pipe) for a specific app/env, use the `--app`/`--env` flags — or run
`lsm set KEY` in a linked directory:

```bash
lsm set --app app --env env KEY   # prompt/pipe for app/env's KEY
```

When only KEY is given and stdin is a **terminal**, lsm prompts for the value and
reads it with **no echo** — the typed secret is never shown on screen and never
lands in shell history:

```bash
lsm set API_KEY
Value for API_KEY:      # typed input is hidden; read once, no confirmation
```

If nothing is entered at the prompt, the command errors (`no value entered`) and
stores nothing.

When only KEY is given and stdin is **piped or redirected**, lsm reads the value
from stdin exactly as if you had passed `-`:

```bash
echo tok | lsm set API_KEY        # same as: echo tok | lsm set API_KEY -
```

### Trailing newline handling

For stdin input (both `lsm set KEY -` and the piped `lsm set KEY` form), lsm
strips a **single** trailing newline. This avoids the common `echo` footgun where
a stored token silently carries an invisible trailing newline:

```bash
echo tok | lsm set API_KEY        # stores "tok", not "tok\n"
```

Only one trailing newline is removed (a lone `\n`, or a `\r\n`). Trailing spaces
or tabs, additional trailing newlines, and interior newlines are all preserved,
so multi-line values remain intact. To store a value that ends in a newline, add
an extra one (e.g. `printf 'tok\n\n'`). The interactive prompt returns the line
without its newline, so no trimming applies there. Values passed directly as
`lsm set KEY VALUE` are stored exactly as given.

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
- Writes real `KEY=VALUE` content to `.env` in the current directory
- Prints masked output to the terminal (e.g., `API_KEY=sk********`)
- Prompts before overwriting an existing `.env` file
- Automatically adds `.env` to `.gitignore` if you're in a git repo (and it's not already ignored)

| Flag | Description |
|------|-------------|
| `--output`, `-o` | Custom output file path (default: `.env`) |

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

## clean

Remove `.env` files from the current directory after verifying all their secrets exist in the encrypted store.

```bash
cd ~/Web/myapp
lsm clean
lsm clean --force    # skip confirmation prompt
```

For each `.env` file found, lsm parses every `KEY=VALUE` pair and checks that the key exists in the encrypted store. Files where all keys are present are safe to remove. Files with any missing keys are skipped with a warning listing the missing keys.

Skips `.env.example`, `.env.sample`, and `.env.template` files (same filtering as `import`).

Deletion uses secure overwrite (zero-fill before remove) so secret values don't linger on disk.

| Flag | Description |
|------|-------------|
| `--force` | Skip confirmation prompt |

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

---

## gh

Manage GitHub Actions secrets from your locally-encrypted lsm store.

`lsm gh` is **directory-bound**: it operates on the app registered for the
current directory via `lsm link`. Unlike the other commands it does **not**
accept `--app` — run it from the project root. It requires the GitHub CLI
(`gh`) to be installed and authenticated (`gh auth login`).

> **Write-only API.** GitHub's secrets API cannot return secret values. Secret
> values can never be pulled back from GitHub; `lsm gh status` shows secret
> names and update timestamps only.

Common flags:

- `--repo OWNER/REPO` — target repository (default: parsed from the `origin`
  remote of the current directory).
- `--gh-env <name>` — target a GitHub Actions **environment's** secrets instead
  of the repository-level secrets.
- `--env <name>` — which local environment to read from (resolves from `--env`,
  then `.lsm.yaml`, then the global default env).

### gh push

Push every secret in the local store to GitHub Actions.

```bash
lsm gh push                        # set repo Actions secrets
lsm gh push --gh-env production     # set a GitHub environment's secrets
lsm gh push --repo acme/widget      # override the target repo
lsm gh push --prune                 # delete remote secrets not present locally
lsm gh push -y                       # skip confirmation prompts (alias: --force)
```

Behavior:

- Prints the secret **names** (never values) and a count, then prompts for
  confirmation. `-y`/`--force` skips the prompt; a non-terminal invocation
  without `--force` is refused rather than run silently.
- Each value is streamed to `gh secret set` on **stdin** — values never appear
  in the process arguments, and no plaintext temp file is written.
- No backup file is created.
- `--prune` deletes GitHub secrets that are not present locally. It lists
  exactly what will be deleted and requires its own confirmation (same
  `--force` / non-terminal rules).
- Emits an audit event recording the names, count, repo, and target — never
  values.

### gh status

Compare the local store with GitHub Actions secrets.

```bash
lsm gh status
lsm gh status --gh-env production
```

Output is grouped into three buckets:

- **In sync** — present both locally and on GitHub (with GitHub's `updatedAt`).
- **Local only** — present locally, would be pushed.
- **Remote only** — present on GitHub but not locally (with GitHub's
  `updatedAt`).

Only names and timestamps are shown — never values.
