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
Where the live energy counter was not read at both ends of an hour, the
dashboards take that hour from this history (`weheat_hourly` in
`tools/dashboards/common.py`). These are separate from live metrics: never splice averages into raw
series or interpolate missing instantaneous readings. The false device readings
are excluded.



## Thermostat no-reading value

Weheat reports -1 for `tRoom` and `tRoomTarget` while it has no reading from the
room thermostat; the collector drops those, so they never appear as -1 °C.
The recovered hourly history (`history_log_t_room_*`, `history_log_t_room_target_*`)
averaged the -1 into real readings, so those hours showed false lows.
Hours whose minimum was -1 are removed from all six series.
Air temperatures pass through -1 for real and are kept.

## API coverage audit

Written by AI on 2026.11.08 by model gpt-6-astra.

Source: [Weheat's own third-party API documentation and debugger](https://api.weheat.nl/third_party/api/debugger).
Only GET requests under `/third_party` are supported here. Its user profile and
notification preferences are account data, not house telemetry.

| What the API gives | Exported before | Exported now | Left out and why |
|---|---|---|---|
| Latest temperatures, power, flow, operation, DHW and cooling codes | Nullable `log_*` metrics | Same | Missing readings and thermostat -1 sentinel |
| Last cooling time | RPC only | `gohome_weheat_log_last_cooling_time_timestamp_seconds` | Absent when source timestamp is missing |
| Outdoor-unit lifetime energy, 11 fields | `energy_total_*_kwh` | Same | None |
| Indoor-unit lifetime energy, six fields | Dropped by dependency | `energy_total_ein_iu_*_kwh` | None when returned |
| Latest hourly energy and mean power, including indoor unit | `energy_log_*` | Same | Missing buckets |
| Lifecycle state and status | RPC only | `gohome_weheat_device_state`, `gohome_weheat_device_status` | None |
| Aggregated log history, including DHW/cooling diagnostics | Dependency omitted newer fields | `GetLogViews` preserves the complete vendor JSON | Historical import awaits deployment of the complete reader |
| Device model, serial, name | Metric labels and RPC | Same | Board/part identifiers and commissioning metadata are not measurements |
| User profile and notification preferences | Not collected | Not collected | Personal account/UI data |
| Raw history | Provider rejects third-party access | Unavailable | Do not bypass the provider restriction |

New metrics use labels `heat_pump_id`, `serial_number`, `name`, `model`.
Indoor-unit lifetime energy uses kWh and suffixes `standby`, `heating`, `dhw`,
`heating_defrost`, `dhw_defrost`, `cooling`. State and status are vendor integer
codes, distinct from the operating mode in `log_state`.

