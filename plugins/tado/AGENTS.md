# Tado - Agent Context

## What This Is
Tado controls cloud‑connected thermostats and heating zones.

## Capabilities
- List zones
- Set target temperature per zone

## Limits
- Temperature range and valid zone IDs depend on the Tado account.
- This MVP stub does not read current temperature.

## Methods
- `ListZones`: returns available zones and IDs.
- `SetTemperature`: sets target temperature for a zone.

## State
- Metrics TBD (gohome_tado_temperature_celsius, etc.).

## Errors
- API auth failures
- Zone not found
