# Observability artifacts for lsm

Optional, drop-in dashboard and alert rules for users running the LGTM stack (Loki + Grafana + optionally Tempo + Mimir) or any OTLP-compatible logs backend.

**lsm does not depend on any of this.** These files are pure configuration — JSON for Grafana, YAML for the Loki ruler. lsm has no knowledge of Grafana or Loki at runtime; it just emits OTLP events when `otlp.enabled: true` in `~/.lsm/config.yaml`. What you do with those events downstream is your stack.

If you don't run an observability stack, ignore this directory.

## Files

- **`grafana-dashboard.json`** — 5-panel dashboard: event rate by type, events by app/env, events by agent marker, non-interactive/no-agent stat, top parent processes.
- **`loki-alerts.yaml`** — 4 alert rules: per-parent burst, non-interactive without agent marker, new agent_marker, aggregate high rate.

## Prerequisites

You need:

1. An **OTLP collector** that ingests log records and forwards them to Loki. The OpenTelemetry Collector with the `otlphttp` receiver and `loki` exporter is the standard combo. Other collectors (Grafana Alloy, Vector with OTLP support) work the same way.
2. **Loki** with the ruler enabled, if you want alerts.
3. **Grafana** with a Loki datasource configured.

The OTLP collector translates OTLP log-record `attributes` into Loki labels. The label names below assume the OpenTelemetry conventions (or close to them); if your collector renames attributes (e.g. via processors), you'll need to adjust the queries.

## Expected Loki labels

The OTLPSink emits 6 attributes on each log record. After the OTLP→Loki translation, you should see these as labels:

| Label | Source | Values |
|---|---|---|
| `service_name` | OTLP resource `service.name` | always `"lsm"` |
| `host_name` | OTLP resource `host.name` | hostname |
| `event` | log record attribute | `get`, `set`, `delete`, etc. |
| `app` | log record attribute | HMAC-hashed app name (12 hex chars) |
| `env` | log record attribute | HMAC-hashed env name (12 hex chars) |
| `tty_present` | log record attribute | `"true"` or `"false"` |
| `agent_marker` | log record attribute | `claude`, `cursor`, `aider`, `continue`, `openhands`, or `none` |

If your collector emits these with different names (e.g. `service.name` becomes `service_name` in some configs but `service.name` literally in others, depending on label-translation settings), update the LogQL queries in both files. The dashboard's `description` panel also notes this.

## Importing the dashboard

**Via Grafana UI:**
1. Dashboards → New → Import
2. Upload `grafana-dashboard.json` (or paste its contents)
3. Select your Loki datasource when prompted (the dashboard uses a `$DS_LOKI` variable so any Loki datasource UID works)
4. Save

**Via provisioning (Grafana Helm chart, Terraform, etc.):**
- Mount or sync `grafana-dashboard.json` into the dashboards provisioning directory
- Loki datasource UID is parameterized via `${DS_LOKI}`, so the dashboard is portable

**Tested against:** Grafana 10.x. Earlier versions may need a lower `schemaVersion` in the JSON.

## Installing the alerts

The rules are Prometheus-format alerting rules consumed by the Loki ruler.

**File-backed ruler:**
- Drop `loki-alerts.yaml` into your configured `rule_path` (Loki ruler config)
- Reload the ruler or restart Loki

**Object-storage-backed ruler:**
- Upload the file into the configured tenant's rules directory in S3/GCS/Azure Blob
- The Loki ruler picks up changes automatically

**Tested against:** Loki 2.9+. Earlier versions support most of the syntax but `__error__` filter handling has been refined.

## What's NOT here

### Local-only events have no remote equivalent

Per [`docs/observability.md`](../docs/observability.md), three event categories are **always local** and never reach Loki:

- `audit.verify.failed` — chain integrity check failure
- `audit.suspicious.flagged` — local detector matches
- `audit.sink.dropped` — events the OTLP queue dropped

You cannot alert on these via Loki. If you want alerting:

- Run **`lsm audit verify`** as a periodic cron / systemd timer. Pipe non-zero exit to whatever your host-local alerting is (`ntfy`, an SSH-to-local-script, etc.).
- Run **`lsm audit suspicious`** the same way. Output is informational; parse it for high-signal patterns.
- Watch for **`audit.sink.dropped`** by monitoring `OTLPSink.Dropped()` — but the recommended path is to size your `queue_cap` correctly and never drop in the first place.

### Time-of-day filtering

LogQL doesn't have native time-of-day predicates (the brainstorm earlier suggested a "3am access" alert; it's not directly expressible). If you need it, either:

- Use a recording rule in Mimir (or Prometheus) to compute "events during off-hours" via `vector()` math, then alert on the recording rule
- Schedule an Alertmanager silence covering business hours and treat any alert outside it as off-hours
- Filter at the dashboard level using Grafana variables for time-of-day

The `LsmNonInteractiveNoAgent` alert is the higher-signal canary for the same threat model (unexplained background access) and works without time-of-day logic.

### Dashboards for `audit verify` / `audit suspicious` output

Those commands write to stdout, not the audit log, so they're not in Loki. Pipe them to your local logging stack if you want dashboards for them (e.g. `lsm audit suspicious --format=json | logger -t lsm-suspicious`), but that's outside lsm's scope.

## Tuning

Default alert thresholds are conservative starting points:

- **Burst** at 50 events/min/parent — appropriate for "real human + one agent". A heavily-scripted workflow may need to bump this to 200 or 500.
- **High event rate** at ~1000/min globally — also assumes mostly-interactive use. Tune up for batch-job-heavy workloads.
- **New agent_marker** uses a 30-day lookback. Shorten if you onboard tools rapidly and don't want the noise.

Edit the YAML directly. Keep the file in your repo / config-management system so changes are reviewable.

## Privacy reminder

App and env names are HMAC-hashed with a per-host salt before leaving the box. You **cannot** correlate dashboard labels back to real app/env names without access to the host's `~/.lsm/observability.salt`. This is intentional — see `docs/observability.md`. If you want plaintext app/env for an internal lsm deployment behind a corporate firewall, configure the OTLPSink with hashing disabled (config grammar TBD; future work).
