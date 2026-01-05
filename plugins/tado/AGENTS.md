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

## CLI quick use
```
gohome-cli tado zones
gohome-cli tado set living-room 20
```

## State
- Metrics: inside temperature, humidity, target setpoint, heating power, power/override status, and last update timestamp per zone; outside temperature and solar intensity at home level.

## Errors
- API auth failures
- Zone not found

## Required Config

Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `tado.bootstrap_file` (required; JSON with client_id/client_secret, optional refresh_token)
- `tado.home_id` (optional override; auto-discovered if omitted)
- `oauth.blob_endpoint` (required)
- `oauth.blob_bucket` (required)
- `oauth.blob_access_key_file` (required)
- `oauth.blob_secret_key_file` (required)
- `oauth.blob_prefix` (optional, default `gohome/oauth`)
- `oauth.blob_region` (optional)

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
