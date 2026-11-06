# Weheat - Agent Context

## What This Is
Weheat controls cloud telemetry for Weheat heat pumps (Sparrow/BlackBird/Flint). This plugin is **read-only** and exposes the full raw API data via gRPC and metrics.

## Capabilities
- List heat pumps
- Fetch heat pump details
- Fetch latest raw log
- Fetch raw logs for a time range
- Fetch aggregated log views for a time range
- Fetch energy totals
- Fetch energy logs for a time range

## Limits
- Cloud polling only (no LAN access).
- OAuth credentials required (Keycloak device flow recommended).
- No control endpoints discovered (read-only).

## Methods
- `ListHeatPumps`: list available heat pumps (optionally filter by device state).
- `GetHeatPump`: fetch a single heat pump by ID.
- `GetLatestLog`: returns raw log JSON for the most recent reading.
- `GetRawLogs`: returns raw log JSON entries for a time range.
- `GetLogViews`: returns aggregated log view JSON entries for a time range.
- `GetEnergyTotals`: returns total energy JSON for a heat pump.
- `GetEnergyLogs`: returns energy log JSON entries for a time range.

## State
- Metrics: `gohome_weheat_log_*` for raw log fields, `gohome_weheat_energy_log_*` for latest hourly energy-log fields, plus energy totals and scrape success.
- Use Grafana for history; raw logs are already time-series friendly.

## Errors
- OAuth auth failures (refresh token invalid/expired)
- Heat pump not found
- API outages or rate limits

## Required Config
Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `weheat.bootstrap_file` (required; JSON with client_id/client_secret + refresh_token)
- `weheat.base_url` (optional; defaults to `https://api.weheat.nl/third_party`)
- `oauth.blob_*` (required for refresh token persistence)

Auth flow:
- `gohome oauth device --provider weheat` (recommended)
- `gohome oauth auth-code --provider weheat --redirect-url <url>`

State file (written by runner): `/var/lib/gohome/weheat-credentials.json`

## API access and historical data

Written by AI on 2026.11.06 by model gpt-6-astra.

Weheat requires third-party clients to use `/third_party/api/v1`; `/api/v1`
returns HTTP 403. The plugin preserves this prefix because the Go dependency
otherwise discards it. Raw historical logs also return 403; hourly logs and
energy history are available in seven-day request windows.

`gohome_weheat_log_*` contains live observations, including instantaneous power
in watts. `gohome_weheat_energy_log_*` contains the latest hourly energy bucket.
Missing fields and failed reads export no device values; cached reads do not
advance success timestamps. Offline logs or logs older than five minutes are
excluded, and hourly energy buckets must be within two hours.

`gohome_weheat_history_log_*` and `gohome_weheat_history_energy_*`, labelled
`interval="Hour"`, contain recovered hourly aggregates for a past date range.
These are separate from live metrics: never splice averages into raw
series or interpolate missing instantaneous readings. The false device readings
are excluded.

