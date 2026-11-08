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
- Motion repeats about once a second while it lasts. Count visits from the
  gaps between notifications, not from the number of notifications.
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
- `WatchEvents`: server stream of `CameraEvent` from the camera's
  `/ISAPI/Event/notification/alertStream`, which gohome keeps open. The
  camera's ten-second keep-alive is not forwarded. A subscriber that falls
  more than 256 events behind is ended with `RESOURCE_EXHAUSTED` and should
  reconnect.

CLI: `gohome-cli hikvision camera`, `gohome-cli hikvision snapshot <file.jpg>`,
`gohome-cli hikvision events` (add `--json` before `hikvision` for JSON).

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
- `gohome_hikvision_event_notifications_total{kind,target}`: active
  notifications since gohome started.
- `gohome_hikvision_last_event_timestamp_seconds{kind,target}`: receipt time.

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
