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
- `p1_homewizard.base_url` (optional; default `http://192.168.1.48`)
- `p1_homewizard.tariff_import_t1_eur_per_kwh`
- `p1_homewizard.tariff_import_t2_eur_per_kwh`
- `p1_homewizard.tariff_export_t1_eur_per_kwh`
- `p1_homewizard.tariff_export_t2_eur_per_kwh`

Example:
```
p1_homewizard {
  base_url: "http://192.168.1.48"
  tariff_import_t1_eur_per_kwh: 0.32
  tariff_import_t2_eur_per_kwh: 0.28
  tariff_export_t1_eur_per_kwh: 0.11
  tariff_export_t2_eur_per_kwh: 0.09
}
```
