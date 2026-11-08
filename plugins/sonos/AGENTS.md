# Sonos - Agent Context

## What This Is
Read-only view of the Sonos system over its local UPnP interface (port 1400, no login).

## Capabilities
- List zones and their players, with each player's home-theatre channels
- Read each zone's transport state, volume, mute and loaded track

## Limits
- Read-only. The plugin sends only these SOAP actions: `ZoneGroupTopology#GetZoneGroupState`,
  `AVTransport#GetTransportInfo`, `AVTransport#GetPositionInfo`, `RenderingControl#GetVolume`,
  `RenderingControl#GetMute`, and the diagnostic Get actions listed below. It cannot play, pause, seek or change volume.
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

## API coverage

Written by AI on 2026.11.08 by model gpt-6-astra.

| What the local API gives | Exported before | Exported now | Left out and why |
| --- | --- | --- | --- |
| Zones, players, channels, software | GetZones | Same | — |
| Playback, volume, mute | Metrics and GetPlayback | Same | — |
| Track metadata, URI, duration, position | GetPlayback | Same, plus numeric track metrics | Titles/URIs stay out of time-series labels |
| Bass, treble, loudness, fixed output | Dropped | Metrics and GetDiagnostics | — |
| Trueplay enabled/available | Dropped | Metrics and GetDiagnostics | — |
| Sub and surround settings, night mode, dialogue enhancement, height level | Dropped | Metrics and GetDiagnostics | Raw settings, not inferred acoustic measurements |
| Audio input format, transport fault/status, play mode, crossfade, sleep timer | Dropped | Metrics and GetDiagnostics | — |
| Current medium track count | Dropped | `queue_tracks` and GetDiagnostics | Not a count of tracks ever played |
| Absolute time and counters, recording mode | Dropped | Not exported | Device reports NOT_IMPLEMENTED or signed-int maximum sentinels |
| Headphone state, decibel volume, running alarm | Dropped | Not exported | Real player rejects GetHeadphoneConnected, GetVolumeDB and GetRunningAlarmProperties |
| Libraries, favourites, playlists, scheduled alarms, artwork | Not collected | Not collected | Content/configuration, not device measurements or observed history |
| Network/account identifiers, LED/button configuration, capabilities | Topology identity only | Same | Static metadata or UI configuration |
| Timestamped playback history | None | None | No playback-history endpoint in the device's UPnP service descriptions; queue is not history |

`GetDiagnostics` returns optional fields and per-action `read_errors`; successful
fields survive another action failing. Each scrape calls only Get actions, 26 local requests per zone including the existing status reads, plus topology.
No cloud requests, subscriptions or control operations. A diagnostic read failure
makes scrape_success 0 and omits the affected fields; successful status reads remain.

New metrics all have prefix `gohome_sonos_` and label `{zone}` unless stated:
- `bass`, `treble`: raw -10 to 10 settings.
- `loudness`, `output_fixed`, `room_calibration_enabled`, `room_calibration_available`,
  `crossfade`, `surround_enabled`, `sub_enabled`, `night_mode`, `speech_enhance_enabled`: 0/1.
- `sub_gain`, `audio_delay`, `dialog_level`, `surround_mode`, `surround_level`,
  `music_surround_level`, `height_channel_level`: raw vendor settings.
- `audio_input_format`: raw HTAudioIn code.
- `track_number`, `queue_tracks`: counts reported by the loaded medium.
- `transport_speed`: playback-speed multiplier.
- `track_duration_seconds`, `media_duration_seconds`, `track_position_seconds`, `sleep_timer_remaining_seconds`:
  seconds; unknown/stream duration and inactive sleep timer absent.
- `play_mode{zone,mode}`, `transport_status{zone,status}`: 1 for the current state.

Source contracts: each player's `/xml/AVTransport1.xml`, `/xml/RenderingControl1.xml`
and `/xml/DeviceProperties1.xml`; EQ names follow
[SoCo](https://github.com/SoCo/SoCo/blob/master/soco/core.py).
