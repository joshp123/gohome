# Daikin Onecta - Agent Context

## What This Is
Daikin Onecta controls cloud‑connected HVAC units.

## Capabilities
- List units
- Read device state
- Toggle power, change mode, set temperature

## Limits
- Requires OAuth refresh state at runtime.
- Refresh tokens rotate; state is mutable and mirrored to blob storage.

## Methods
- `ListUnits`: returns available units and IDs.
- `GetUnitState`: returns raw JSON payload.
- `SetOnOff`: toggles power.
- `SetOperationMode`: sets HVAC mode.
- `SetTemperature`: sets target temperature.

## State
- Metrics include temperature, setpoint, humidity, power/mode, and last update timestamps.

## Required Config

Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `daikin.bootstrap_file` (required; JSON with client_id/client_secret, optional refresh_token)
- `oauth.blob_endpoint` (required)
- `oauth.blob_bucket` (required)
- `oauth.blob_access_key_file` (required)
- `oauth.blob_secret_key_file` (required)
- `oauth.blob_prefix` (optional, default `gohome/oauth`)
- `oauth.blob_region` (optional)

Auth flow:
- Auth-code flow via `gohome oauth auth-code --provider daikin --redirect-url <url>`

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
