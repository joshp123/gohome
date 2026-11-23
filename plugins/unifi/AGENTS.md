# UniFi Network Plugin

Read-only network telemetry from a local UniFi OS / Network controller via `X-API-KEY` (secret file managed by agenix).

## RPCs

- `gohome.plugins.unifi.v1.UniFiService/GetSiteSummary`: returns controller identity, subsystem health (`wan`, `lan`, `wlan`, `www`), adopted device telemetry, and aggregate wired/wireless client counts.

## Metrics

Low-cardinality metrics only (no per-client MAC label series):

- `gohome_unifi_subsystem_ok{site,subsystem}`
- `gohome_unifi_subsystem_tx_bytes_per_second{site,subsystem}`
- `gohome_unifi_subsystem_rx_bytes_per_second{site,subsystem}`
- `gohome_unifi_wan_latency_milliseconds{site}`
- `gohome_unifi_wan_uptime_seconds{site}`
- `gohome_unifi_clients_active{site,connection="wired|wireless"}`
- `gohome_unifi_device_up{site,device,model,type}`
- `gohome_unifi_device_cpu_percent{site,device,model,type}`
- `gohome_unifi_device_memory_percent{site,device,model,type}`
- `gohome_unifi_device_temperature_celsius{site,device,model,type}`
- `gohome_unifi_device_uptime_seconds{site,device,model,type}`
- `gohome_unifi_device_clients{site,device,model,type}`
- `gohome_unifi_device_satisfaction_percent{site,device,model,type}`
- `gohome_unifi_scrape_success`
