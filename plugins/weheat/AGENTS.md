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
- Metrics: `gohome_weheat_log_*` for raw log fields, plus energy totals and scrape success.
- Use Grafana for history; raw logs are already time-series friendly.

## Errors
- OAuth auth failures (refresh token invalid/expired)
- Heat pump not found
- API outages or rate limits

## Required Config
Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `weheat.bootstrap_file` (required; JSON with client_id/client_secret + refresh_token)
- `weheat.base_url` (optional; defaults to `https://api.weheat.nl`)
- `oauth.blob_*` (required for refresh token persistence)

Auth flow:
- `gohome oauth device --provider weheat` (recommended)
- `gohome oauth auth-code --provider weheat --redirect-url <url>`

State file (written by runner): `/var/lib/gohome/weheat-credentials.json`
