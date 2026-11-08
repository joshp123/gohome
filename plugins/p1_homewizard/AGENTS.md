# P1 Homewizard - Agent Context

## What This Is
P1 Homewizard exposes Dutch smart meter telemetry over a local HTTP API.

## Capabilities
- Read device info
- Read current power + energy totals, per-phase voltage when supplied, and Wi-Fi signal strength
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

## Wi-Fi strength, and why there is no voltage

Written by AI on 2026.11.08 by model claude opus 5.5.

- `gohome_p1_homewizard_wifi_strength_percent`: Wi-Fi signal strength, percent. No plugin labels; exported only
  when present in a successful `/api/v1/data` response; zero is a valid reading.

This meter reports no mains voltage: neither `/api/v1/data` nor its raw DSMR telegram carries a voltage field
(checked read-only on 8 Nov 2026). It reports only sag and swell counts. Mains voltage comes from the Growatt
inverter (`gohome_growatt_inverter_grid_voltage_volts`) while it runs.

A DSMR 4.2 meter supplies L1 power/current and sag/swell counts,
but no voltage in either `/api/v1/data` or the raw telegram. Voltage must remain
absent for this meter; do not estimate it from power/current or assume 230 V.
Sag/swell counters count events, not hours or voltage magnitude.
Wi-Fi strength is the only other numerical measurement in its live data
response that is not an electricity reading; device/network identifiers are
metadata and are not exported as metrics.

[HomeWizard's measurement contract](https://api-documentation.homewizard.com/docs/v1/measurement/)
makes voltage optional, depending on the connected smart meter.
[The local API has no history](https://api-documentation.homewizard.com/docs/introduction/):
the device does not store it, so voltage cannot be backfilled from the meter.

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

