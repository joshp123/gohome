# Casambi lights - Agent Context

Written by AI on 2026.11.07 by model claude opus 5.5.

## What This Is
Casambi Bluetooth lights, read through the light bridge: a separate service that
owns the only Bluetooth session to the Casambi network and also serves the
lights to Apple Home. It exposes a read-only gRPC API; this plugin calls it and
serves the same `CasambiService` from gohome.

## Capabilities
- List every light the bridge serves to Apple Home: id (Casambi unit UUID),
  name, online, on and when the light last reported. While a light is on:
  brightness (0 to 100) and, for colour lights, hue (degrees), saturation
  (percent) and Casambi's colour/white balance. For drivers with one, the raw
  overheat indicator.
- List the wall switches: battery level, online, and per button the presses the
  bridge has seen since it started, with the time of the last one.
- List the native Casambi scenes the bridge exposes, and whether each is available.

## Limits
- Read-only. There is no RPC that switches, dims or recalls a scene, here or on
  the bridge. Control will come later, behind gohome's gate for commands that
  change the house.
- State is what the bridge last heard from each light. Lights report when they
  change, so an old `last_report_time` on an online light is normal.
- `on` is absent until a light has reported since the bridge started.
  Brightness and colour are absent while a light is off: an off light has none.
- Hue is absent at saturation 0, where it has no meaning.
- Casambi names but does not document the scales of `white_colour_balance`
  (raw 0 to 63, default 31), `overheat_indicator` (raw 0 to 3, default 0) and a
  switch's `battery_level` (raw 0 to 255). They are reported raw.
- Button numbers are casambi-bt's. Press counts restart at 0 when the bridge
  restarts, and presses while the bridge has no Bluetooth session are missed.

## Methods
- `ListLights`: every light, plus whether the bridge holds its Bluetooth session.
- `ListScenes`: configured scenes; `available` is true when the bridge is
  connected and the Casambi network has the scene.
- `ListSwitches`: every wall switch, plus whether the bridge holds its Bluetooth session.

Errors from the bridge pass through unchanged. `UNAVAILABLE` means gohome could
not reach the bridge.

## State
Metrics (Prometheus):
- `gohome_casambi_scrape_success`: 1 when the bridge answered both reads and
  held its Bluetooth session; 0 otherwise.
- `gohome_casambi_last_success_timestamp_seconds`
- `gohome_casambi_bluetooth_connected`: present whenever the bridge answered.
- `gohome_casambi_light_online{light_id,name}`
- `gohome_casambi_light_on{light_id,name}`: online lights only.
- `gohome_casambi_light_brightness_percent{light_id,name}`: online lights that are on.
- `gohome_casambi_light_hue_degrees{light_id,name}`: online colour lights that are on.
- `gohome_casambi_light_saturation_percent{light_id,name}`: online colour lights that are on.
- `gohome_casambi_light_white_colour_balance{light_id,name}`: online colour/white lights that are on.
- `gohome_casambi_light_overheat_indicator{light_id,name}`: online lights whose driver has one.
- `gohome_casambi_light_last_report_timestamp_seconds{light_id,name}`
- `gohome_casambi_switch_online{switch_id,name}`
- `gohome_casambi_switch_battery_level{switch_id,name}`: once the switch has reported.
- `gohome_casambi_switch_last_report_timestamp_seconds{switch_id,name}`
- `gohome_casambi_switch_button_presses_total{switch_id,name,button}`: counter.
- `gohome_casambi_switch_button_last_press_timestamp_seconds{switch_id,name,button}`

When the bridge is unreachable or has no Bluetooth session, no light or switch
values are emitted: old state is never presented as current.

## Required Config
Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `casambi.bridge_address` (required): host:port of the bridge's API.

Example:
```
casambi {
  bridge_address: "bridge.example.ts.net:50061"
}
```

Nix: `services.gohome.plugins.casambi.bridgeAddress`. The bridge serves the API
only when its own configuration enables it.
