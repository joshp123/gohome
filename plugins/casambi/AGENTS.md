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
  brightness (0 to 100) and the colour of the light it gives. For drivers
  with one, the raw overheat indicator.
- List the wall switches: battery level, online, and per button what the
  Casambi configuration says it does (the light, group or scene it addresses)
  and the presses the bridge has seen since it started, with the time of the
  last one.
- List the last 100 switch presses since the bridge started: per press the
  button's configured action, whether it was held, and the lights whose on/off
  state, brightness or colour changed in the few seconds after it, even if
  they changed back.
- List the native Casambi scenes the bridge exposes, and whether each is available.

## Limits
- Read-only. There is no RPC that switches, dims or recalls a scene, here or on
  the bridge. Control will come later, behind gohome's gate for commands that
  change the house.
- State is what the bridge last heard from each light. Lights report when they
  change, so an old `last_report_time` on an online light is normal.
- `on` is absent until a light has reported since the bridge started.
  Brightness and colour are absent while a light is off: an off light has none.
- Colour is the colour of the light, as sRGB at full value; how bright is
  brightness. Casambi does not report the colour temperature of a light's
  white LEDs, so the bridge's configuration states it per light: it is
  configured, not measured, and a light without one reports no colour. A
  light with only white LEDs gives that white.
- Lights with colour and white LEDs, such as a CBU-PWM4 in Casambi's
  "RGB/White" profile: "Four channel RGB fixture with White. The ratio between
  RGB and White is selected with mixer" (CBU-PWM4 manual). The mixer runs 0
  to 63; at 0 only the white LEDs are lit, whatever colour the colour LEDs are
  set to (seen on real lights). The bridge mixes the white with the colour
  LEDs' colour, taking mixer/63 as the colour LEDs' share of the light.
  Casambi does not document how the mixer divides the light between 0 and 63.
- Casambi names but does not document the scales of `overheat_indicator` (raw
  0 to 3, default 0) and a switch's `battery_level` (raw 0 to 255). They are
  reported raw.
- Button numbers are casambi-bt's. Press counts restart at 0 when the bridge
  restarts, and presses while the bridge has no Bluetooth session are missed.
- Configured actions come from the network configuration the bridge cached
  when it first signed in to Casambi. A switch reconfigured in the Casambi app
  keeps its old action here until that cache is refreshed. Casambi does not
  document its action type codes; `casambi_action_type` is reported raw.
- The configuration numbers buttons from 0; the bridge takes configured button
  i to be button i+1. A press's `light_changes` show whether that holds.
- A press's `light_changes` are what the lights reported, not what the press
  caused: anything else that changed a light in the same seconds shows too,
  and a light with a switch-off delay can change after the observation ends.

## Methods
- `ListLights`: every light, plus whether the bridge holds its Bluetooth session.
- `ListScenes`: configured scenes; `available` is true when the bridge is
  connected and the Casambi network has the scene.
- `ListSwitches`: every wall switch, plus whether the bridge holds its Bluetooth session.
- `ListSwitchPresses`: recent presses, oldest first; a press appears once its
  observation has ended, about five seconds after the button's last event.

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
- `gohome_casambi_light_colour_info{light_id,name,colour}`: always 1, for
  online lights that are on and have a configured white. `colour` is
  `#rrggbb`, the colour of the light at full brightness. One series per light
  at a time; a new one starts when the colour changes.
- `gohome_casambi_light_overheat_indicator{light_id,name}`: online lights whose driver has one.
- `gohome_casambi_light_last_report_timestamp_seconds{light_id,name}`
- `gohome_casambi_switch_online{switch_id,name}`
- `gohome_casambi_switch_battery_level{switch_id,name}`: once the switch has reported.
- `gohome_casambi_switch_last_report_timestamp_seconds{switch_id,name}`
- `gohome_casambi_switch_button_presses_total{switch_id,name,button,action}`:
  counter, present from 0 for every configured button. `action` is what the
  configuration says the button does: `light <name>`, `group <name>`,
  `scene <name>`, `none` (no target), `other` (a target gohome cannot name) or
  `not configured` (a button the configuration does not list).
- `gohome_casambi_switch_button_last_press_timestamp_seconds{switch_id,name,button,action}`:
  once the button has been pressed.

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
