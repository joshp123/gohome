# Tado - Agent Context

## What This Is
Tado controls cloud‑connected thermostats and heating zones.

## Capabilities
- List zones
- Set target temperature per zone

## Limits
- Requires OAuth refresh state at runtime.
- Refresh tokens rotate; state is mutable and mirrored to blob storage.
- Temperature range and valid zone IDs depend on the Tado account.

## Methods
- `ListZones`: returns available zones and IDs.
- `SetTemperature`: sets target temperature for a zone.

## State
- Metrics: inside temperature, humidity, target setpoint, heating power, power/override status, and last update timestamp per zone; outside temperature and solar intensity at home level.

## Errors
- API auth failures
- Zone not found

## Required Config

Environment variables:
- `GOHOME_TADO_BOOTSTRAP_FILE` (required; JSON with client_id/client_secret, optional refresh_token)
- `GOHOME_TADO_HOME_ID` (optional override)
- `GOHOME_OAUTH_BLOB_ENDPOINT` (required)
- `GOHOME_OAUTH_BLOB_BUCKET` (required)
- `GOHOME_OAUTH_BLOB_ACCESS_KEY_FILE` (required)
- `GOHOME_OAUTH_BLOB_SECRET_KEY_FILE` (required)
- `GOHOME_OAUTH_BLOB_PREFIX` (optional, default `gohome/oauth`)
- `GOHOME_OAUTH_BLOB_REGION` (optional)

Auth flow:
- Device flow via `gohome oauth device --provider tado`

State file (written by runner): `/var/lib/gohome/tado-token.json`
```json
{
  "schema_version": 1,
  "client_id": "...",
  "client_secret": "...",
  "refresh_token": "...",
  "scope": "offline_access"
}
```
