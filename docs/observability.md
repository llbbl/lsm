# Observability — decisions

This document captures the locked-in decisions for how lsm's audit events leave the box when remote sinks ship. These decisions exist primarily to support a future OTLP/Loki sink (see #8) — they should be reflected in any `Sink` implementation that forwards events off-machine.

The local `FileSink` is not bound by these rules. It writes the full canonical `Event` (with `Hash` / `Prev`) to `~/.lsm/audit.jsonl`. The decisions below apply only to **remote** sinks.

## Why decide now

Loki and similar log aggregators are unforgiving about cardinality once labels are in flight. Pick wrong on day one and the index melts, retention costs explode, or you re-onboard your whole audit-event corpus into a new schema. The cheap fix is to pick well at design time; the expensive fix is migration.

The redaction story has the same shape. Once events ship, they may be retained, indexed, and (in cloud-hosted observability stacks) exposed to vendor employees under their TOS. App names and secret-key names are sensitive even when the secret values themselves aren't.

## 1. Loki label cardinality

Labels are stable, low-cardinality dimensions Loki indexes. Everything else lives in the log line body.

### Labels (low cardinality, indexed)

| Label | Type | Expected distinct values | Why |
|---|---|---|---|
| `event` | string | ~10–20 (`get`, `set`, `delete`, `snapshot`, `audit.verify.failed`, ...) | Primary query dimension |
| `app` | string (hashed; see redaction) | typically <100 per user | Per-app dashboards and alerts |
| `env` | string (hashed; see redaction) | typically <10 (`dev`, `staging`, `prod`, ...) | Per-environment alerts |
| `host` | string | 1–3 per user | Multi-machine correlation |
| `tty_present` | bool | 2 | Cheap interactive-vs-not split for alerting |
| `agent_marker` | string | <10 (`claude`, `cursor`, `aider`, `continue`, `openhands`, `none`) | "Which AI tool ran this?" — load-bearing for the suspicious-detection use case |

### Log-line fields (high cardinality or sensitive)

These ride in the event body, NOT as labels:

- `seq` — monotonic, every value unique
- `timestamp` (RFC3339Nano) — every value unique
- `actor.parent_comm` — many process names; queries `group_by` this rather than indexing it
- `actor.ppid` — high cardinality, low query value
- `actor.uid` — usually constant per host but no benefit as a label
- `actor.cwd` — high cardinality, sensitive (filesystem layout)
- `actor.tty` — full device path lives here; the `tty_present` label is the indexed bool form
- `fields.*` — arbitrary user-provided data

### Explicitly NOT shipped

These stay local-only:

- `hash` and `prev` — the chain is a local integrity record. Anchoring the chain off-machine is a separate, future mechanism (e.g., periodic `{seq, hash}` checkpoint events), not the per-row chain bytes. Shipping `hash`/`prev` adds storage cost and leaks zero useful information to remote consumers.
- `schema_version` — implied by sink configuration; no value as either label or field. (Reconsider if we ever ship to a generic OTLP collector consuming multiple lsm versions simultaneously.)

## 2. Redaction policy

Default redaction transforms applied by remote sinks before emit. Allowlist semantics: a field must be explicitly listed below to leave the box.

### Identifiers (hashed, not omitted)

To support per-app and per-env dashboards/alerts without leaking the actual names:

- `app` → `hmac_sha256(host_salt, app_name)`, first 12 hex chars
- `env` → `hmac_sha256(host_salt, env_name)`, first 12 hex chars

The 12-char truncation gives 48 bits — easily enough to avoid collisions at lsm's scale, short enough to read in dashboards. Each host has its own salt, so the same `app` name on two machines produces different hashed labels. This is intentional: cross-host correlation isn't the goal; per-host pattern detection is.

**Salt management:** `~/.lsm/observability.salt` — 32 random bytes generated on first use, persisted with `0600`. Same lifecycle as the encryption key. Lose it and dashboards re-key (acceptable — recreate dashboards, no data lost). Stricter modes (`0400`, `0500`, `0700`) are also accepted; lsm only reads the salt once at startup and never rewrites it after creation.

### Reshaped fields

- `actor.tty` → `tty_present: bool` (label). The full device path is dropped, not hashed. The presence/absence is the load-bearing fact.
- `actor.cwd` → omitted by default. Configurable to ship `basename(cwd)` for users who want it; never ship the full path.

### Shipped as-is

- `actor.ppid`, `actor.uid`, `actor.parent_comm`, `actor.agent_marker` — operationally useful, not directly sensitive.
- `seq`, `timestamp`, `event` — required for any meaningful queries.

### Redacted completely

- `fields.key` — when an event records that a specific secret name was touched (`DATABASE_URL`, `API_KEY`, ...), the key name itself is sensitive and stays local. Remote events emit `fields.key_present: true` or similar, not the value.
- `fields.value` — never. Secret values must never leave the box.
- Any `fields.*` not explicitly allowlisted by the sink's redaction config — omit by default. Allowlist > denylist for this layer.

### Not shipped

- `hash`, `prev` (see Section 1)
- The full `Event` JSON as a single blob — log lines should be the structured projection above, not the raw on-disk event. Otherwise `hash`/`prev` would leak through.

## 3. `LocalOnly` events

The `Event.LocalOnly` field (added in #4) signals "never ship this off-machine, regardless of the sink's allowlist." Two categories use it:

### Always `LocalOnly`

Events about the audit system itself stay local. Shipping them would either be circular (an audit-verify failure event would also be subject to verification...) or actively counterproductive (telling a remote attacker that local integrity is broken).

- `audit.verify.failed` — `lsm audit verify` reported a chain break
- `audit.suspicious.flagged` — local detector matched a row (the underlying event may or may not ship per its own `LocalOnly` flag)
- `audit.sink.dropped` — events the sink couldn't ship (network failure, queue overflow)

### Caller-flagged `LocalOnly`

Any emit site can set `LocalOnly: true` on an event it considers too sensitive for remote shipping, regardless of category. Use cases anticipated but not yet implemented:

- Key generation / rotation events (`init`, `rotate`) — reveal cryptographic lifecycle
- Identity unlock events (future agent broker work)
- Any event explicitly marked by the user via a future per-event config

### Implementation contract for remote sinks

A remote sink **must**:

1. Check `e.LocalOnly` before any transformation. If true, drop the event silently. No metric counter; no log line; the existence of the event is itself the information that's local-only.
2. Check the event-name allowlist. Events starting with `audit.*` are always-local by convention; the sink should hard-code this in addition to honoring the `LocalOnly` flag.
3. Apply redaction (Section 2) only to events that pass steps 1 and 2.

The local `FileSink` ignores `LocalOnly` entirely. The flag is purely a contract between callers and remote-bound sinks.

## Configuration shape

The exact YAML/TOML shape lives with the `OTLPSink` implementation work (#8). At minimum, the config must let the user:

- Toggle remote shipping on/off
- Override the allowlist (add a `fields.*` path the user wants shipped despite the default)
- Override the redaction defaults (e.g., disable app/env hashing for an internal lsm instance behind a corporate firewall)
- Disable specific event types (e.g., skip `get` events to reduce volume, ship only `set`/`delete`)

The defaults documented above are the safe-by-default starting point. Users who want less redaction must opt in explicitly.

## Open questions, deferred

These are intentionally unresolved here — the right time to decide is when the corresponding code lands:

- **Chain anchoring.** Periodic `{host, seq, hash, timestamp}` checkpoint events to a remote sink would let an external observer detect local-side log truncation. Worth doing; not part of the first OTLP cut.
- **Field-level redaction config.** Today this doc says "allowlist," but the actual config grammar (regex? path expressions? a fixed enum?) gets settled in #8.
- **Batching semantics.** Per-event vs per-N-events vs per-T-seconds shipping. Affects user-perceived freshness and remote-side cost. Decide alongside OTLPSink implementation, not here.
- **OTLP attributes vs body.** OTLP logs let you put fields in either; the choice has cost and queryability implications on the consumer side. Punt to implementation time.
