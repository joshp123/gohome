# Casambi lights - Agent Context

Written by AI on 2026.11.07 by model claude opus 5.5.

## What This Is
Casambi Bluetooth lights, read through the light bridge: a separate service that
owns the only Bluetooth session to the Casambi network and also serves the
lights to Apple Home. It exposes a read-only gRPC API; this plugin calls it and
serves the same `CasambiService` from gohome.

## Capabilities
- List every light the bridge serves to Apple Home: id (Casambi unit UUID),
  name, online, on, brightness (0 to 100) and when the light last reported.
- List the native Casambi scenes the bridge exposes, and whether each is available.

## Limits
- Read-only. There is no RPC that switches, dims or recalls a scene, here or on
  the bridge. Control will come later, behind gohome's gate for commands that
  change the house.
- State is what the bridge last heard from each light. Lights report when they
  change, so an old `last_report_time` on an online light is normal.
- `on` and `brightness_percent` are absent until a light has reported since the
  bridge started.

## Methods
- `ListLights`: every light, plus whether the bridge holds its Bluetooth session.
- `ListScenes`: configured scenes; `available` is true when the bridge is
  connected and the Casambi network has the scene.

Errors from the bridge pass through unchanged. `UNAVAILABLE` means gohome could
not reach the bridge.

## State
Metrics (Prometheus):
- `gohome_casambi_scrape_success`: 1 when the bridge answered and held its
  Bluetooth session; 0 otherwise.
- `gohome_casambi_last_success_timestamp_seconds`
- `gohome_casambi_bluetooth_connected`: present whenever the bridge answered.
- `gohome_casambi_light_online{light_id,name}`
- `gohome_casambi_light_on{light_id,name}`: online lights only.
- `gohome_casambi_light_brightness_percent{light_id,name}`: online lights only.
- `gohome_casambi_light_last_report_timestamp_seconds{light_id,name}`

When the bridge is unreachable or has no Bluetooth session, no light values are
emitted: old state is never presented as current.

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
