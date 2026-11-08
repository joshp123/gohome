# AirGradient - Agent Context

## What This Is
AirGradient exposes local LAN air-quality telemetry from AirGradient ONE/Open Air devices via their built-in HTTP API.

## Capabilities
- Read current air quality (PM, CO2, temp, humidity, VOC/NOx, particle counts)
- Read raw JSON snapshot
- Read device configuration (read-only)
- Read OpenMetrics payload from the device

## Limits
- Read-only (no config writes in GoHome)
- Local LAN access only
- mDNS hostnames require LAN mDNS; use static IP if GoHome is off-LAN

## Methods
- `GetCurrent`: returns structured readings from `/measures/current`.
- `GetSnapshot`: returns raw JSON from `/measures/current`.
- `GetConfig`: returns raw JSON from `/config`.
- `GetMetrics`: returns raw OpenMetrics from `/metrics`.

## State
Metrics (Prometheus):
- `gohome_airgradient_scrape_success`
- `gohome_airgradient_last_success_timestamp_seconds`
- `gohome_airgradient_last_update_timestamp_seconds`
- `gohome_airgradient_openmetrics_scrape_success`
- `gohome_airgradient_config_ok`
- `gohome_airgradient_post_ok`
- `gohome_airgradient_info{serial,model,firmware,led_mode}`
- `gohome_airgradient_wifi_rssi_dbm`
- `gohome_airgradient_co2_ppm`
- `gohome_airgradient_pm01_ugm3`
- `gohome_airgradient_pm02_ugm3`
- `gohome_airgradient_pm10_ugm3`
- `gohome_airgradient_pm02_compensated_ugm3`
- `gohome_airgradient_pm01_standard_ugm3`
- `gohome_airgradient_pm02_standard_ugm3`
- `gohome_airgradient_pm10_standard_ugm3`
- `gohome_airgradient_pm003_count_per_dl`
- `gohome_airgradient_pm005_count_per_dl`
- `gohome_airgradient_pm01_count_per_dl`
- `gohome_airgradient_pm02_count_per_dl`
- `gohome_airgradient_pm50_count_per_dl`
- `gohome_airgradient_pm10_count_per_dl`
- `gohome_airgradient_temperature_celsius`
- `gohome_airgradient_temperature_compensated_celsius`
- `gohome_airgradient_humidity_percent`
- `gohome_airgradient_humidity_compensated_percent`
- `gohome_airgradient_tvoc_index`
- `gohome_airgradient_tvoc_raw`
- `gohome_airgradient_nox_index`
- `gohome_airgradient_nox_raw`
- `gohome_airgradient_boot_count`
- `gohome_airgradient_satellite_temperature_celsius{satellite_id}`
- `gohome_airgradient_satellite_humidity_percent{satellite_id}`
- `gohome_airgradient_satellite_wifi_rssi_dbm{satellite_id}`

## Errors
- HTTP timeouts/unreachable device
- Invalid JSON or OpenMetrics payloads

## Required Config
Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `airgradient.base_url` (required)

Example:
```
airgradient {
  base_url: "http://192.0.2.43"
}
```

## Historical data integrity

Written by AI on 2026.11.06 by model gpt-6-astra.



## API coverage audit

Written by AI on 2026.11.08 by model gpt-6-astra.

Compared firmware 3.7.0 live `/measures/current` and `/metrics` with the
[manufacturer local API](https://github.com/airgradienthq/arduino/blob/master/docs/local-server.md).

| What the API gives | Exported before | Exported now | Left out and why |
|---|---|---|---|
| CO₂, particulate mass and counts, temperature, humidity, compensated readings, VOC/NOx indices and raw signals, Wi-Fi signal | All fields | All fields | None |
| Measurement cycle count (`boot`, `bootCount`) | `boot_count` | Same | Duplicate `boot` alias; this counts measurement cycles, not restarts |
| Serial, model, firmware, LED mode | Info metric | Same | None |
| Cloud configuration fetch and upload health | `config_ok`, `post_ok` | Same, absent when missing | None |
| OpenMetrics sensor values | JSON measurements; raw `GetMetrics` RPC | Same | Duplicate measurements with different rounding; do not mix with JSON readings |
| Configuration | Raw `GetConfig` RPC | Same | Display, calibration and network configuration are not measurements |
| Satellites | Temperature, humidity, signal strength | Same | None present in the live response |
| Local history | No endpoint documented | None | Local API exposes current readings only; cloud history requires separate cloud credentials |


The live collector probe also passed the reachable → unreachable → reachable
sequence: readings disappeared on failure and returned on recovery, with no
false advance of the last-success timestamp.
