# Tado - Agent Context

## What This Is
Tado controls cloud‑connected thermostats and heating zones.

## Capabilities
- List zones
- Set target temperature per zone

## Limits
- Temperature range and valid zone IDs depend on the Tado account.
- Requires OAuth refresh credentials at runtime.

## Methods
- `ListZones`: returns available zones and IDs.
- `SetTemperature`: sets target temperature for a zone.

## State
- Metrics: inside temperature + humidity per zone.

## Errors
- API auth failures
- Zone not found
- Invalid token file format

## Required Config

Environment variables:
- `GOHOME_TADO_TOKEN_FILE` (required)
- `GOHOME_TADO_HOME_ID` (optional override)

Token file JSON:
```json
{
  "client_id": "...",
  "client_secret": "...",
  "refresh_token": "...",
  "scope": "..." 
}
```
