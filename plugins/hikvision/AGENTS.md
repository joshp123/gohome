# Hikvision camera - Agent Context

## What This Is
One Hikvision IP camera on the home network, read over its local ISAPI (HTTP
with Digest authentication). Built for Hikvision cameras with on-camera people
and vehicle classification (AcuSense).

**This plugin never writes to the camera.** Every request it sends is an HTTP
GET to one of the resources listed under Methods. It cannot change a setting,
switch the infrared or any light, sound an alarm, trigger an output, reboot
the camera or update its firmware, and it has no method that could.

## Capabilities
- Read the camera's identity and firmware.
- Read whether motion, line-crossing and intrusion detection are switched on,
  and which targets (people, vehicles) each is filtered to.
- Take one JPEG snapshot from the main stream.
- Watch the camera's own events (motion, line crossing, intrusion, tamper and
  others) as they happen.

## Limits
- Read-only, as above. Changing detection settings needs the camera's web page
  or Hik-Connect app, and the owner's approval.
- What the camera calls a person or vehicle is its own judgement. It is known
  to raise false alarms; do not treat a motion event as a person.
- Motion events never say what moved, even with motion filtered to people and
  vehicles: their target is `DETECTION_TARGET_UNSPECIFIED`. Line-crossing and
  intrusion events say `human` or `vehicle`.
- Motion detection reacts to leaves, branches and shadows moving in the wind,
  despite its people-and-vehicles filter. Most motion events are not people.
- While an event lasts the camera repeats a notification about once a second
  and never says it has ended. gohome turns the repeats into one event: it
  starts with the first notification and ends after 10 s without a repeat.
  gohome reports the end only when a later message, such as the camera's
  ten-second keep-alive, shows the stream is still alive, so 10 to 20 s
  after the event was last seen. A silent connection never ends an event.
  Each line-crossing or intrusion rule is its own event.
- Events that happen while the event stream is disconnected are lost; the
  camera does not replay them.
- The camera's clock (`camera_time`) can drift. Use `received_at` to correlate
  with other sources.
- No video. RTSP streams (`rtsp://<camera>:554/Streaming/Channels/101` main,
  `/102` sub) are for a recorder such as Frigate, not this plugin.

## Methods
Service `gohome.plugins.hikvision.v1.HikvisionService`:
- `GetCamera`: identity, firmware, supported event types, motion settings,
  line-crossing and intrusion rules, and event-stream status. Reads
  `/ISAPI/System/deviceInfo`, `/ISAPI/System/capabilities`,
  `/ISAPI/Event/capabilities`,
  `/ISAPI/System/Video/inputs/channels/1/motionDetection`,
  `/ISAPI/Smart/LineDetection/1` and `/ISAPI/Smart/FieldDetection/1` with their
  `/capabilities`.
- `GetSnapshot`: one full-resolution JPEG from
  `/ISAPI/Streaming/channels/101/picture`.
- `WatchEvents`: server stream of `CameraEvent`, one `ACTIVE` when an event
  starts and one `INACTIVE` when it ends, from the camera's
  `/ISAPI/Event/notification/alertStream`, which gohome keeps open. A new
  subscriber first gets the starts of events already under way. An end
  carries the event's last notification, so its times say when the event was
  last seen. The camera's ten-second keep-alive is not forwarded.
  `UNAVAILABLE` while gohome is not connected to the camera's event stream,
  and when it loses it: events under way then have no end, because nobody
  can tell when they ended. A subscriber that falls more than 256 events
  behind is ended with `RESOURCE_EXHAUSTED`. Reconnect after either.

CLI: `gohome-cli hikvision camera`, `gohome-cli hikvision snapshot <file.jpg>`,
`gohome-cli hikvision events` (add `--json` before `hikvision` for JSON).

## HTTP picture
<!-- Written by AI on 2026.11.08 by model gpt-6-astra. -->
`GET /hikvision/snapshot.jpg` (also `HEAD`) serves the main-stream JPEG.
Grafana uses `/gohome/hikvision/snapshot.jpg` on the hub; nginx forwards this
route to the Hikvision instance in `services.gohome.remotePlugins` when set.
The hub route accepts only tailnet clients. The house service must listen on
its tailnet address, as configured on the house host.

Pictures are fetched on request and shared in memory for at most five seconds,
then released. Nothing is saved to disk. `Cache-Control: no-store` prevents
browser and proxy caching. `X-Snapshot-Captured-At` is UTC RFC 3339 with
fractional seconds: the time gohome finished fetching the picture, matching
`GetSnapshot.captured_at`; ISAPI provides no exposure timestamp. Cache hits
retain that timestamp. A failed refresh returns HTTP 502 without a picture;
an unconfigured plugin returns 503. Neither returns the expired picture.
The dashboard must reload the image URL when it refreshes; a static HTML image
does not poll on its own.

## State
Metrics (Prometheus):
- `gohome_hikvision_scrape_success`: 1 if the last read of identity and
  detection settings succeeded. This is the camera's reachability.
- `gohome_hikvision_last_success_timestamp_seconds`
- `gohome_hikvision_info{model,firmware_version,firmware_build}`
- `gohome_hikvision_detection_enabled{detection="motion|line_crossing|intrusion"}`
- `gohome_hikvision_alert_stream_connected`
- `gohome_hikvision_alert_stream_last_message_timestamp_seconds`: includes
  keep-alives, so it is at most ten seconds old while the stream is healthy.
- `gohome_hikvision_events_total{kind,target}`: events that started since
  gohome started, one per event however long it lasted.
- `gohome_hikvision_events_active{kind,target}`: events happening now (more
  than 1 when several rules fire at once). Absent while the event stream is
  disconnected. Only kinds and targets seen since gohome started appear.
- `gohome_hikvision_last_event_timestamp_seconds{kind,target}`: when the
  latest event started.

`info` and `detection_enabled` are absent when the last read failed, rather
than repeating old values. `kind` and `target` come from the fixed
`EventKind` and `DetectionTarget` vocabularies; no camera text or image is
ever a label.

## Errors
- `UNAVAILABLE` with "camera rejected the credentials": the username or
  password is wrong. The plugin then stops asking for 30 minutes, because the
  camera locks out a login after repeated wrong passwords.
- `UNAVAILABLE` with a network error: the camera is unreachable.
- Event stream "no message from the camera for 1m0s": the connection went
  silent; gohome reconnects with backoff up to one minute.

## Required Config
Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `hikvision.base_url` (required): the camera's origin, with its HTTP port
  when that is not 80
- `hikvision.credentials_file` (required): one line, `username:password`

Example:
```
hikvision {
  base_url: "http://192.0.2.20"
  credentials_file: "/run/agenix/gohome-hikvision"
}
```

## Device status and API coverage
<!-- Written by AI on 2026.11.08 by model gpt-6-astra. -->

`GetDeviceStatus` reads `/ISAPI/System/status`, `/ISAPI/ContentMgmt/Storage`,
`/ISAPI/Streaming/status` and `/ISAPI/ContentMgmt/record/tracks`. It returns
optional typed measurements, storage states and recording configuration.
An omitted number stays absent. A failed resource makes the RPC unavailable.
Metrics keep each resource independent: only that resource's readings disappear
on failure, and `read_success{resource}` becomes 0. They recover on the next
successful read, without retaining old readings.

All names below have prefix `gohome_hikvision_`; all are gauges.

| Metric and labels | Unit and meaning |
| --- | --- |
| `read_success{resource}` | 0/1 for `status`, `storage`, `streaming`, `recording` |
| `uptime_seconds` | Seconds since camera startup |
| `device_time_seconds` | Camera clock as Unix seconds; compare with query time in Grafana |
| `reboot_count` | Camera-reported reboot count; reset/persistence semantics unspecified |
| `cpu_usage_percent{cpu}` | Percent utilisation; CPU index starts at 1 |
| `memory_usage_raw{memory}` | Unconverted `memoryUsage`; memory index starts at 1 |
| `memory_available_raw{memory}` | Unconverted `memoryAvailable`; memory index starts at 1 |
| `storage_capacity_megabytes{storage,id}` | Total space in ISAPI MB; storage is `hdd` or `nas` |
| `storage_free_megabytes{storage,id}` | Free space in ISAPI MB |
| `storage_status{storage,id,status}` | 1 for the reported state, such as `ok`, `error`, `offline`, `unformatted` |
| `storage_property{storage,id,property}` | 1 for the access property, such as `RW`, `RO`, `Redund` |
| `streaming_sessions` | Number of live video sessions; no client addresses are exported |
| `recording_enabled{track}` | 0/1: track configured as enabled, not proof of recording now |
| `recording_loop_enabled{track}` | 0/1: overwriting older recordings permitted |
| `recording_schedule_enabled{track}` | 0/1: recording schedule enabled |

Memory units must not be inferred from the generic manual. It labels both
fields MB, but on a tested camera the values matched percent used and free
KiB respectively, and neither matched the Linux `MemAvailable` value. The raw
metrics deliberately
do not claim a universal conversion. Storage MB also retains the vendor unit
rather than assuming decimal or binary bytes.

The collector adds four authenticated GETs to the four settings GETs per
scrape. Each uses a Digest challenge plus an authenticated request: 16 HTTP
requests per 15-second scrape, plus the existing persistent alert stream.
Live collection of the four status resources completed in under a second.
No cloud API or login is involved. Snapshots remain demand-driven and cached
for five seconds. Capability scans, diagnostic dumps and archive/log searches
are audit operations, never scrape operations.

### API assessment

| What ISAPI gives | Exported before | Exported now | Left out and why |
| --- | --- | --- | --- |
| Identity, firmware, capabilities | Identity and supported events over gRPC; identity metric | Same | Serial/network identifiers stay out of metric labels |
| Motion and smart detection settings | gRPC settings, enabled metrics | Same | Geometry, arming schedules and encoder/image settings configure behaviour; they are not readings |
| Device clock, uptime, CPU, memory, reboot count | None | `GetDeviceStatus` and metrics above | No memory unit guess |
| Disk/NAS status, capacity, free space, access property | None | `GetDeviceStatus` and metrics above | Mount paths, encryption options and format choices are configuration |
| Recording-track enable, overwrite and schedule flags | None | `GetDeviceStatus` and metrics above | Track description contains configured codec/rate, not measured throughput; GUIDs and schedule tables are configuration |
| Video sessions | None | Current session count over gRPC and metrics | Client IP addresses identify viewers, not device performance |
| Main-stream JPEG | `GetSnapshot` | Also cached HTTP JPEG | No periodic pictures or disk archive |
| Event type, state, time, target, rule/channel | Episode events over gRPC and metrics | Same, plus `active_post_count`, `event_description`, `channel_name` in `CameraEvent` | Transport IP/MAC/port/protocol repeat the configured source; multipart classification/image work belongs to the detection implementation |
| Temperature, fans, pressure, tamper sensors | None | None | Absent from this camera's `DeviceStatus`; do not synthesise zeros |
| PTZ/lens/dome wear and face/mixed-target counters | None | None | Generic status includes all-zero fields for unsupported hardware/features on this fixed camera; they are not established measurements |
| Physical input/output ports | None | None | Live input and output lists are empty |
| Per-track recording status, HDD SMART status, video-input status, network-interface status, working status | None | None | Probed GETs return 404; `System/Hardware` says `notSupport` |
| Recorded video archive | None | None | Live read-only search returns clip intervals and playback URIs, not historical device measurements. Video storage/playback remains on the camera; GoHome is not a recorder |
| Camera logs | None | None | Retained ring is almost entirely API/config audit operations, not readings |
| Diagnostic dump | None | None | Debug text, kernel logs and process/network details are not a stable measurement API; used only to check memory-unit claims |

`CameraEvent.active_post_count` is the camera's notification sequence, not an
extra event counter. Episode ends retain the latest notification's metadata.
The stream's inactive-video-loss messages are keep-alives and stay out of
`WatchEvents`; they already advance the stream's last-message timestamp.
Classified people/vehicle detection is a separate implementation; this status
collector does not change its configuration or event accounting.

### Evidence and repeatable checks

Sanitised live response shapes are in `testdata/System_status.xml`,
`ContentMgmt_Storage.xml`, `Streaming_status.xml`, and
`ContentMgmt_record_tracks.xml`. They carry no camera credentials, client
addresses, track GUIDs or device serial numbers. Live `TestLiveDeviceStatus`
reads the four real resources through the production collector and prints
its emitted metrics. Set `HIKVISION_BASE_URL` and a protected
`HIKVISION_CREDENTIALS_FILE`; it never changes a setting.

Tests cover resource failure/recovery, omitted readings,
wrong response types and event metadata retention.


Sources: [Hikvision ISAPI developer guide](https://raw.githubusercontent.com/loozhengyuan/hikvision-sdk/master/resources/isapi.pdf),
sections XML_DeviceStatus, XML_hdd, XML_EventNotificationAlert and ContentMgmt
search/logSearch; [pyHik client](https://github.com/mezz64/pyHik/blob/master/pyhik/isapi.py).
Actual GET responses, rather than advertised capabilities alone, determine
which resources this collector polls.
