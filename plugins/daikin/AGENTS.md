# Daikin Onecta - Agent Context

## What This Is
Daikin Onecta controls cloud‑connected Daikin HVAC units via the Onecta API.

## Capabilities
- List units (device id + name + climate control management point id)
- Fetch raw device JSON for exploration
- Set on/off state
- Set operation mode (raw Daikin value)
- Set temperature setpoint for an operation mode
- Export telemetry metrics (room/outdoor temps, setpoints, on/off, op mode, rate limits)

## Limits
- Requires OAuth2 refresh state at runtime.
- Refresh tokens rotate; state is mutable and mirrored to blob storage.
- Operation mode and setpoint names must match values in the device JSON.
- Rate limits are enforced by the Daikin cloud API.
- GET responses can be stale right after a PATCH; we reuse the last cached gateway payload for ~10s after a patch.

## Troubleshooting
- **Auth flow**: run `gohome oauth auth-code --provider daikin --redirect-url <url>` to obtain a fresh refresh token.
- **Scope mismatch**: emits `gohome_oauth_scope_mismatch_total{provider="daikin"}` and requires reauth.
- **invalid_grant**: refresh tokens are single‑use/rotating; reauth via the runner and replace state.

## Methods
- `ListUnits`: returns units with `climate_control_id` for PATCH requests.
- `GetUnitState`: returns raw JSON payload for a unit (use to discover valid modes/setpoints).
- `SetOnOff`: sets `onOffMode` (usually `on` / `off`).
- `SetOperationMode`: sets `operationMode` (raw Daikin mode string).
- `SetTemperature`: sets `temperatureControl` at `operationModes/{operation_mode}/setpoints/{setpoint}`.

## State
- Metrics:
  - `gohome_daikin_cloud_connected`
  - `gohome_daikin_scrape_success`
  - `gohome_daikin_on_off`
  - `gohome_daikin_operation_mode`
  - `gohome_daikin_room_temperature_celsius`
  - `gohome_daikin_outdoor_temperature_celsius`
  - `gohome_daikin_room_humidity_percent`
  - `gohome_daikin_setpoint_celsius`
  - `gohome_daikin_error_state`
  - `gohome_daikin_warning_state`
  - `gohome_daikin_caution_state`
  - `gohome_daikin_holiday_mode_active`
  - `gohome_daikin_rate_limit`
  - `gohome_daikin_rate_limit_remaining`
  - `gohome_daikin_rate_limit_retry_after_seconds`
  - `gohome_daikin_rate_limit_reset_after_seconds`
  - `gohome_daikin_last_status_code`

## Errors
- OAuth token refresh failures
- Rate limit errors
- Invalid management point IDs or operation modes

## Required Config

Environment variables:
- `GOHOME_DAIKIN_BOOTSTRAP_FILE` (required; JSON with client_id/client_secret, optional refresh_token)
- `GOHOME_OAUTH_BLOB_ENDPOINT` (required)
- `GOHOME_OAUTH_BLOB_BUCKET` (required)
- `GOHOME_OAUTH_BLOB_ACCESS_KEY_FILE` (required)
- `GOHOME_OAUTH_BLOB_SECRET_KEY_FILE` (required)
- `GOHOME_OAUTH_BLOB_PREFIX` (optional, default `gohome/oauth`)
- `GOHOME_OAUTH_BLOB_REGION` (optional)

State file (written by runner): `/var/lib/gohome/daikin-credentials.json`
```json
{
  "schema_version": 1,
  "client_id": "...",
  "client_secret": "...",
  "refresh_token": "...",
  "scope": "openid onecta:basic.integration"
}
```
