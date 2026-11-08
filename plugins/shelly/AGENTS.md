# Shelly shutters - Agent Context

## What This Is
Outside roller shutters, each driven by a Shelly 2PM Gen3 relay running the community shelly-homekit firmware in cover mode. gohome reads them over the home network with one unauthenticated HTTP call per relay, `GET /rpc/Shelly.GetInfoExt`. Apple Home controls the same relays over HomeKit; gohome does not touch that.

## Capabilities
- Read each shutter's position, target position, whether the motor is running, and whether the relay is calibrated.
- Read each relay's chip temperature and Wi-Fi signal.

## Limits
- Read-only. gohome cannot open, close or stop a shutter yet.
- Position is the relay's estimate from motor run time, not a sensor. It is only reported when the relay is calibrated.
- The firmware has no push notifications: state is read when metrics are scraped (every 15 s) or when `ListCovers` is called.
- No live motor power or energy: this firmware does not expose them for covers.
- Relays get addresses by DHCP. A read fails, rather than reporting the wrong shutter, if the relay at a configured address reports a different device ID.

## Methods
- `ListCovers`: reads every configured shutter live. Each `Cover` has `device_id`, `name`, `address`, and either `read_error` or the state: `read_at`, `position_percent` and `target_position_percent` (0 closed, 100 open; absent when unknown), `moving`, `calibrated`, `state` (the relay's state word: `idle`, `move`, `rampup`, `moving`, `stop`, `stopping`, `precal0`..`postcal1` while calibrating, or `error`).

Example: `gohome-cli call gohome.plugins.shelly.v1.ShellyService/ListCovers`

## State
Metrics (Prometheus), each labelled `device_id` and `name`:
- `gohome_shelly_scrape_success` (1 read OK, 0 failed)
- `gohome_shelly_last_success_timestamp_seconds`
- `gohome_shelly_cover_position_percent`
- `gohome_shelly_cover_target_position_percent`
- `gohome_shelly_cover_moving`
- `gohome_shelly_cover_calibrated`
- `gohome_shelly_device_temperature_celsius`
- `gohome_shelly_wifi_rssi_dbm`

When a read fails, only `scrape_success` (0) and the last success time are emitted for that relay. Position and the rest are absent, never the previous value.

## Errors
- `request ...: dial tcp ...`: relay unreachable (off, out of Wi-Fi range, or its address changed).
- `relay at <address> is "<id>", expected "<id>"`: a different relay now has that address. Fix the address in the Nix config, or give the relays fixed IPs on the router.
- `relay <id> has no cover component`: the relay is not in cover mode.

## Required Config
Canonical config file: `/etc/gohome/config.pbtxt` (textproto). Set through `services.gohome.plugins.shelly.covers` in Nix.

Each cover needs `device_id` (as the relay reports it), `address` and `name`; device IDs and names must be unique.

Example:
```
shelly {
  covers { device_id: "Shelly2PMG3-AAAAAAAAAAAA" address: "192.0.2.30" name: "Shutter 1" }
}
```

## Live check
Read-only probe of one real relay, an unreachable address and a wrong device ID. It proves a failed read removes the shutter's state from metrics and degrades registry health:
```sh
SHELLY_LIVE_ADDRESS=192.0.2.30 SHELLY_LIVE_DEVICE_ID=Shelly2PMG3-AAAAAAAAAAAA SHELLY_DOWN_ADDRESS=127.0.0.1:9 \
  devenv shell -- go test ./plugins/shelly -run TestLiveSourceFailureRemovesReadings -v -count=1
```
