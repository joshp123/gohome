# Hikvision camera - Agent Context

## What This Is
One Hikvision IP camera on the home network, read over its local ISAPI (HTTP
with Digest authentication). Built for Hikvision cameras with on-camera people
and vehicle classification (AcuSense).

**This plugin never writes to the camera.** Every request it sends is an HTTP
GET to one of the resources listed under Methods, except the two searches
(`/ISAPI/ContentMgmt/logSearch` and `/ISAPI/ContentMgmt/search`), which ISAPI
takes as a POST of search terms and which only read; and RTSP playback of the
camera's own recording, which only reads. It cannot change a setting, switch
the infrared or any light, sound an alarm, trigger an output, reboot the
camera or update its firmware, and it has no method that could.

## Capabilities
- Read the camera's identity and firmware.
- Read whether motion, line-crossing and intrusion detection are switched on,
  and which targets (people, vehicles) each is filtered to.
- Take one JPEG snapshot from the main stream.
- **Alarms**: the alarms the camera raises, as it logs them: the ones its
  detection filter accepts (for motion filtered to people and vehicles, a
  person or vehicle). List them, and watch them as they are logged.
- Read one picture from the camera's own recording at a past moment, such as
  an alarm's start.
- Watch the raw alert stream (motion, line crossing, intrusion, tamper and
  others) as it happens.

## Alarms and raw events
The camera reports motion in two places, and they are not the same thing:
- **Alarms** (`ListAlarms`, `WatchAlarms`, `gohome_hikvision_alarms_*`) come
  from the camera's alarm log. An alarm is logged only when the camera's
  detection filter accepts it (for motion: a person or vehicle), and each
  logged alarm is a start and a stop line, usually five seconds apart. Use
  these for "someone or something is there".
- **Raw events** (`WatchEvents`, `gohome_hikvision_events_*`) come from the
  alert stream, which carries every reaction of the motion detector before
  that filter: wind, branches, shadows and headlights. There can be a
  hundred or more raw motion events for each alarm. Use them only to study
  the detector.

## Limits
- Read-only, as above. Changing detection settings needs the camera's web page
  or Hik-Connect app, and the owner's approval.
- What the camera calls a person or vehicle is its own judgement. It is known
  to raise false alarms; do not treat a motion event as a person.
- Neither source says whether a motion alarm was a person or a vehicle: the
  log line is `motionStart`, and raw motion events have target
  `DETECTION_TARGET_UNSPECIFIED`. Raw line-crossing and intrusion events say
  `human` or `vehicle`.
- An alarm reaches watchers up to one poll interval (`alarm_poll_seconds`,
  default 5, start to start) plus one read (typically under 0.2 s) after the
  camera makes it readable in its log; the camera's own delay in logging it
  comes on top. `seen_at` minus `started_at` is the whole delay. During a camera outage they arrive when the log
  is readable again, with their true start times; `seen_at` shows the delay.
  Alarms logged before gohome started are history: `ListAlarms` returns them,
  `WatchAlarms` never sends them as new.
- The camera's log is a ring buffer; it keeps the last few weeks of alarms.
- While an event lasts the camera repeats a notification about once a second
  and never says it has ended. gohome turns the repeats into one event: it
  starts with the first notification and ends after 10 s without a repeat.
  gohome reports the end only when a later message, such as the camera's
  ten-second keep-alive, shows the stream is still alive, so 10 to 20 s
  after the event was last seen. A silent connection never ends an event.
  Each line-crossing or intrusion rule is its own event.
- Raw events that happen while the event stream is disconnected are lost; the
  camera does not replay them. Alarms are not lost this way.
- The camera's clock (`camera_time`) can drift. Use `received_at` to correlate
  with other sources.
- No live video. RTSP streams (`rtsp://<camera>:554/Streaming/Channels/101`
  main, `/102` sub) are for a recorder such as Frigate. Recorded pictures are
  read one at a time from the camera's memory card, which records only what
  its recording schedule covers: `NOT_FOUND` outside it.

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
- `ListAlarms(since, until)`: the alarms the camera logged, oldest first,
  paired from start and stop lines. Reads `/ISAPI/ContentMgmt/logSearch`.
- `WatchAlarms`: server stream of `Alarm`, one `ACTIVE` when a new alarm's
  start is logged and one `INACTIVE` with `ended_at` and `duration` when its
  stop is logged (both in one read when the whole alarm was logged between
  two reads), beginning with announced alarms under way. Each alarm is sent
  once. While the camera is unreachable the stream stays open, and alarms
  logged meanwhile arrive when it is readable again, with their true times
  and a later `seen_at`; `gohome_hikvision_alarm_log_readable` says whether
  the log is being read. `RESOURCE_EXHAUSTED` for a subscriber more than 256
  alarms behind.
- `GetFrame(at)`: one full-resolution JPEG from the camera's recording at the
  second `at`. Finds the recording with `/ISAPI/ContentMgmt/search` (track
  101), then plays it back over RTSP from that second and decodes the first
  picture with ffmpeg. About two seconds. `NOT_FOUND` when nothing was
  recorded then; `FAILED_PRECONDITION` when ffmpeg is missing.
- `WatchEvents`: raw alert-stream events: server stream of `CameraEvent`, one `ACTIVE` when an event
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
`gohome-cli hikvision alarms [24h]`, `gohome-cli hikvision watch-alarms`,
`gohome-cli hikvision frame <RFC 3339 time> <file.jpg>`,
`gohome-cli hikvision events` (raw; add `--json` before `hikvision` for JSON).

## HTTP pictures
<!-- Written by AI on 2026.11.08 by model gpt-6-astra and on 2026.11.09 by model claude opus 5.5. -->
`GET /hikvision/snapshot.jpg` (also `HEAD`) serves the main-stream JPEG.
`GET /hikvision/frame.jpg?at=<RFC 3339>` serves the recorded picture at that
second, as `GetFrame`, with `X-Frame-At`. `GET /hikvision/alarm.jpg` serves
the recorded picture at the start of the latest logged alarm, with
`X-Frame-At` and `X-Alarm-Started-At`. These two answer 404 when nothing was
recorded at that second (or no alarm is known), 400 for a bad `at`, 503 when
ffmpeg is missing, 502 when the camera fails. Recorded pictures are kept in
memory for five minutes (the last eight), so dashboard refreshes do not replay
the recording; nothing is written to disk.
Grafana uses `/gohome/hikvision/<picture>` on the hub; nginx forwards these
routes, query string included, to the Hikvision instance in
`services.gohome.remotePlugins` when set. The hub routes accept only tailnet
clients. The house service must listen on its tailnet address, as configured
on the house host.

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
- `gohome_hikvision_alarms_total{kind}`: alarms the camera logged since
  gohome started (counter; history before the start is not counted).
- `gohome_hikvision_last_alarm_start_timestamp_seconds{kind}`: when the latest
  logged alarm started, camera clock, including alarms from before gohome
  started. Absent while the alarm log is unreadable. Its distinct values are
  the alarm start times. Before its first live sample it may hold imported
  history, one sample per alarm at the alarm's start (RECOVERY.md).
- `gohome_hikvision_alarm_log_readable`: 1 if gohome read the alarm log in the
  last minute.
- `gohome_hikvision_alarm_log_last_read_timestamp_seconds`
- `gohome_hikvision_events_total{kind,target}`: raw alert-stream events that
  started since gohome started, one per event however long it lasted.
- `gohome_hikvision_events_active{kind,target}`: raw events happening now (more
  than 1 when several rules fire at once). Absent while the event stream is
  disconnected. Only kinds and targets seen since gohome started appear.
- `gohome_hikvision_last_event_timestamp_seconds{kind,target}`: when the
  latest raw event started.

`info` and `detection_enabled` are absent when the last read failed, rather
than repeating old values. `kind` and `target` come from the fixed
`EventKind` and `DetectionTarget` vocabularies (`motion`, `other`, ...); no
camera text or image is ever a label.

## Errors
- `UNAVAILABLE` with "camera rejected the credentials": the username or
  password is wrong. The plugin then stops asking for 30 minutes, because the
  camera locks out a login after repeated wrong passwords.
- `UNAVAILABLE` with a network error: the camera is unreachable.
- Event stream "no message from the camera for 1m0s": the connection went
  silent; gohome reconnects with backoff up to one minute.
- Alarm log read errors are logged once until the log is readable again;
  gohome keeps trying every poll interval.

## Required Config
Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `hikvision.base_url` (required): the camera's origin, with its HTTP port
  when that is not 80
- `hikvision.credentials_file` (required): one line, `username:password`
- `hikvision.alarm_poll_seconds` (optional, default 5, at least 1): seconds
  between reads of the alarm log, start to start. Nix:
  `services.gohome.plugins.hikvision.alarmPollSeconds`.

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
metrics deliberately do not claim a universal conversion. Storage MB also
retains the vendor unit rather than assuming decimal or binary bytes.

The collector adds four authenticated GETs to the four settings GETs per
scrape. Each uses a Digest challenge plus an authenticated request: 16 HTTP
requests per 15-second scrape, plus the existing persistent alert stream.
Live collection of the four status resources completed in under a second.
No cloud API or login is involved. Snapshots remain demand-driven and cached
for five seconds. The alarm log is read every `alarm_poll_seconds` by its own
poller (one search, two HTTP requests), not by the scrape. Capability scans and
diagnostic dumps are audit operations, never scrape operations.

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
| Recorded video archive | None | One picture at a requested second (`GetFrame`, picture routes) | Video stays on the camera; GoHome is not a recorder |
| Camera logs | None | Alarm lines (`Alarm/motionStart`, `motionStop`) as alarms and metrics | Operation and information lines are API/config audit records, not readings |
| Diagnostic dump | None | None | Debug text, kernel logs and process/network details are not a stable measurement API; used only to check memory-unit claims |

`CameraEvent.active_post_count` is the camera's notification sequence, not an
extra event counter. Episode ends retain the latest notification's metadata.
The stream's inactive-video-loss messages are keep-alives and stay out of
`WatchEvents`; they already advance the stream's last-message timestamp.

### Evidence and repeatable checks

Sanitised live response shapes are in `testdata/System_status.xml`,
`ContentMgmt_Storage.xml`, `Streaming_status.xml`, and
`ContentMgmt_record_tracks.xml`. They carry no camera credentials, client
addresses, track GUIDs or device serial numbers. Tests cover resource
failure/recovery, omitted readings, wrong response types and event metadata
retention; for alarms, pairing, paging, history not replayed, each alarm sent
once, catch-up after an outage, and absence while the log is unreadable.

Opt-in live probes read the real camera through the production code and never
change a setting. Set `HIKVISION_BASE_URL` and a protected
`HIKVISION_CREDENTIALS_FILE`, then: `TestLiveDeviceStatus` prints the status
metrics; `TestLiveAlarmLog` (with `HIKVISION_ALARMS_SINCE`, e.g. `240h`)
prints every alarm, to compare one-for-one with the camera's own log page;
`TestLiveRecordedFrame` (ffmpeg on `PATH`) reads the latest alarm's recorded
picture and prints only its size and timing.

Sources: [Hikvision ISAPI developer guide](https://raw.githubusercontent.com/loozhengyuan/hikvision-sdk/master/resources/isapi.pdf),
sections XML_DeviceStatus, XML_hdd, XML_EventNotificationAlert and ContentMgmt
search/logSearch; [pyHik client](https://github.com/mezz64/pyHik/blob/master/pyhik/isapi.py).
Actual GET responses, rather than advertised capabilities alone, determine
which resources this collector polls.
