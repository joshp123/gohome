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
- `airgradient.base_url` (optional; default `http://192.168.1.243`)

Example:
```
airgradient {
  base_url: "http://192.168.1.243"
}
```
