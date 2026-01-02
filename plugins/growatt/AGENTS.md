# Growatt - Agent Context

## What This Is
Growatt provides cloud telemetry for solar/inverter systems via the ShinePhone app.

## Capabilities
- List plants
- Read plant energy overview

## Limits
- Cloud polling only (no LAN IP).
- Requires a valid Growatt OpenAPI token.
- Plant selection required if multiple plants exist.

## Methods
- `ListPlants`: returns available plants and IDs.
- `GetPlantStatus`: returns current power and energy totals.

## State
- Metrics: current power, today/month/year/total energy, last update timestamp, scrape success.

## Errors
- Invalid/expired token
- Plant not found
- API rate limits or outages

## Required Config

Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `growatt.token_file` (required; path to API token)
- `growatt.region` (optional; defaults to `other_regions`)
- `growatt.plant_id` (optional; required if multiple plants exist)

Token format (file contents):
```
<token>
```
