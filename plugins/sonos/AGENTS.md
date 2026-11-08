# Sonos - Agent Context

## What This Is
Read-only view of the Sonos system over its local UPnP interface (port 1400, no login).

## Capabilities
- List zones and their players, with each player's home-theatre channels
- Read each zone's transport state, volume, mute and loaded track

## Limits
- Read-only. The plugin sends only these SOAP actions: `ZoneGroupTopology#GetZoneGroupState`,
  `AVTransport#GetTransportInfo`, `AVTransport#GetPositionInfo`, `RenderingControl#GetVolume`,
  `RenderingControl#GetMute`. It cannot play, pause, seek or change volume.
- Volume is the coordinator's, which for a home-theatre zone is the zone's volume.
- Local network only.

## Methods
- `GetZones`: zones, coordinator, and players (UUID, name, host, software version, channels, satellite).
  Channels: `LF,RF` front (the Arc), `LR`/`RR` rear surrounds, `SW` sub.
- `GetPlayback`: per zone: state, volume (0-100), muted, and the track when one is loaded.
  A track URI starting `x-sonos-htastream:` is TV audio over HDMI, so a PLAYING zone with that URI means the TV is on.

## State
Metrics (Prometheus), read on every scrape:
- `gohome_sonos_scrape_success`
- `gohome_sonos_last_success_timestamp_seconds`
- `gohome_sonos_zone_reachable{zone}`
- `gohome_sonos_transport_state{zone,state}` (1 for the current state of: playing, paused, stopped, transitioning, no_media)
- `gohome_sonos_volume{zone}`
- `gohome_sonos_muted{zone}`

Track titles are only in `GetPlayback`, never in metric labels. A failed read emits no values
for what failed, so a gap means unknown, not stopped.

## Errors
- `Unavailable`: the player did not answer, or answered with a UPnP fault.

## Required Config
Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `sonos.base_url` (required): any player's UPnP address; the topology names the rest.

Example:
```
sonos {
  base_url: "http://192.0.2.10:1400"
}
```
