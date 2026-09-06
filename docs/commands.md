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

---

## audit

Inspect and verify the audit log. The log is a hash-chained JSON Lines file at `~/.lsm/audit.jsonl` (override with `--file`, or point every command at a different lsm directory with `--dir`).

Every subcommand accepts `--format`:

| Value | Behavior |
|-------|----------|
| `text` | One columnar line per event |
| `json` | One JSON object per line, the raw stored event |
| `auto` | Default. `text` when stdout is a terminal, `json` when piped |

The `auto` default means `lsm audit tail` is readable by eye, while piping it into `jq` gets machine-readable input without needing a flag.

The text line format is:

```
2026-09-06T14:23:11Z  seq=42  event=gh.push  myapp/production  zsh/claude  ttys004
```

The columns are timestamp (UTC, RFC3339), sequence number, event name, `app/env`, `parent_comm/agent_marker`, and the controlling TTY. A missing value renders as `-`.

Each stored event carries a schema version, sequence number, timestamp, event name, app, env, an actor block, optional event-specific `fields`, and the `prev`/`hash` pair that forms the chain. The actor block records the parent process ID, parent command name, TTY, working directory, agent marker, and UID.

### audit tail

Show the most recent events, optionally following the file live.

```bash
lsm audit tail
lsm audit tail -n 100
lsm audit tail --follow
```

| Flag | Description |
|------|-------------|
| `--num`, `-n` | Number of events to show (default: 20) |
| `--follow`, `-f` | Watch the file and print new events as they arrive |
| `--file` | Path to `audit.jsonl` (default: `~/.lsm/audit.jsonl`) |

### audit show

Show one event in full, by sequence number.

```bash
lsm audit show 42
```

Unlike the one-line formats, this prints every field including the working directory, the `fields` map, and both chain hashes.

| Flag | Description |
|------|-------------|
| `--file` | Path to `audit.jsonl` (default: `~/.lsm/audit.jsonl`) |

### audit query

Filter and stream events. All filters combine with AND.

```bash
lsm audit query --app myapp --env production
lsm audit query --event gh.push --since 7d
lsm audit query --tty absent --agent-marker claude
lsm audit query --seq-from 100 --seq-to 200
```

| Flag | Description |
|------|-------------|
| `--app` | Exact match on app |
| `--env` | Exact match on env |
| `--event` | Exact match on event name |
| `--parent-comm` | Exact match on `actor.parent_comm` |
| `--agent-marker` | Exact match on `actor.agent_marker`, for example `claude` |
| `--tty` | `present` (TTY non-empty) or `absent` (TTY empty) |
| `--since` | Events at or after this time |
| `--until` | Events strictly before this time |
| `--seq-from` | Events with sequence at or above this |
| `--seq-to` | Events with sequence at or below this |
| `--file` | Path to `audit.jsonl` (default: `~/.lsm/audit.jsonl`) |

`--since` and `--until` each accept an RFC3339 timestamp, the literal `now`, or a duration that is subtracted from now. Durations take the suffixes `s`, `m`, `h`, `d`, and `w`, so `--since 24h` means the last day and `--since 2w` the last fortnight.

### audit verify

Walk the log and confirm the hash chain is intact. This is what detects tampering: editing or deleting a line breaks the chain from that point on.

```bash
lsm audit verify
```

On success it prints the event count:

```
OK: chain valid (events=128)
```

A missing log and an empty log are both reported as nothing to verify rather than treated as failures. A broken chain exits non-zero.

| Flag | Description |
|------|-------------|
| `--file` | Path to `audit.jsonl` (default: `~/.lsm/audit.jsonl`) |

### audit suspicious

Scan the log with heuristic detectors and report matching events. Informational only: it always exits 0, even when it flags something. An event matching several detectors reports all of them.

```bash
lsm audit suspicious
lsm audit suspicious --hours 09:00-18:00
lsm audit suspicious --burst-threshold 20 --burst-window 30s
```

| Detector | Fires when |
|----------|-----------|
| `outside_hours` | The event timestamp falls outside the working-hours window |
| `burst` | More than `--burst-threshold` events from one `parent_comm` land within `--burst-window` |
| `new_parent_comm` | The `parent_comm` was not seen in records older than `--lookback` |
| `non_interactive_no_agent` | The actor has neither a TTY nor an agent marker |

| Flag | Description |
|------|-------------|
| `--hours` | Working-hours window in UTC (default: `07:00-23:00`) |
| `--burst-threshold` | Event count that triggers the burst detector (default: 50) |
| `--burst-window` | Window for burst detection (default: `1m`) |
| `--lookback` | Age beyond which a `parent_comm` counts as already seen (default: `30d`) |
| `--file` | Path to `audit.jsonl` (default: `~/.lsm/audit.jsonl`) |

The `new_parent_comm` detector needs history to be meaningful. When the log spans less than `--lookback`, it is skipped and a note is written to stderr.

When nothing matches, the command prints `no suspicious events found`.

Flagged events are prefixed with their reasons:

```
[outside_hours,burst] 2026-09-06T03:14:07Z  seq=91  event=gh.push  myapp/production  zsh  -
```

See [Observability](observability.md) for what these events contain and which fields leave the machine when OTLP shipping is enabled.

---

## version

Print version, commit, and build information.

```bash
lsm version
lsm --version
```

Both forms print the same detailed block:

```
lsm v0.11.2
  commit:  3f105cb...
  built:   2026-07-05T01:23:03Z
  go:      go1.25.7
  os/arch: darwin/arm64
```

Released binaries carry the git tag, stamped in at build time. A binary from `go install github.com/llbbl/lsm/cmd/lsm@v0.11.2` reports that same version from Go's module metadata. Anything built straight from a checkout reports `devel-<sha>`, with `+dirty` appended when the tree has uncommitted changes, falling back to `dev` when no build information is available at all.

Commit and build time come from the embedded build info and are omitted when absent.

