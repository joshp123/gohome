# P1 Homewizard - Agent Context

## What This Is
P1 Homewizard exposes Dutch smart meter telemetry over a local HTTP API.

## Capabilities
- Read device info
- Read current power + energy totals
- Read raw DSMR telegram

## Limits
- Read-only (no control)
- Local LAN access only
- Costs are derived from configured tariffs and may be inaccurate

## Methods
- `GetInfo`: returns product metadata.
- `GetSnapshot`: returns raw `/api/v1/data` JSON.
- `GetTelegram`: returns raw DSMR telegram text.

## State
- Metrics: active power, import/export totals (kWh), tariff, current, sag/swell counts, power-fail counts, costs, scrape health.

## Required Config

Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `p1_homewizard.base_url` (required)
- `p1_homewizard.tariff_import_t1_eur_per_kwh`
- `p1_homewizard.tariff_import_t2_eur_per_kwh`
- `p1_homewizard.tariff_export_t1_eur_per_kwh`
- `p1_homewizard.tariff_export_t2_eur_per_kwh`

Example:
```
p1_homewizard {
  base_url: "http://192.0.2.48"
  tariff_import_t1_eur_per_kwh: 0.30
  tariff_import_t2_eur_per_kwh: 0.25
  tariff_export_t1_eur_per_kwh: 0.10
  tariff_export_t2_eur_per_kwh: 0.08
}
```

## Historical data integrity

Written by AI on 2026.11.06 by model gpt-6-astra.

