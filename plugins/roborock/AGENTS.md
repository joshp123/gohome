# Roborock - Agent Context

## What This Is
Roborock controls Roborock vacuums (Qrevo S) via local TCP after a one-time cloud bootstrap.

## Capabilities
- List vacuums
- Read status and consumables
- Start, pause, stop, dock, locate
- Set fan speed
- Set mop mode and mop intensity
- Clean zone / segment, go to position
- Configure DND schedule
- Reset consumables

## Limits
- Camera/live view is not supported on Qrevo S (no camera hardware)
- No remote-control joystick in v1
- Cloud login is required for bootstrap only
- Cloud fallback is optional and not enabled by default (status-only fallback; commands still require local)
- Fan/mop numeric codes are device-specific; prefer CLI-friendly modes unless you know the codes

## Live Map (v2)
- Live map requires MQTT access via Roborock cloud (map RPC channel).
- HTTP endpoint: `http://<gohome-host>:8080/roborock/map.png?device_name=<device>`
- The PNG shows the rooms, the dock (green square), the vacuum (white circle
  with an arrow for where it faces), the path of the last run (white lines),
  no-go zones and virtual walls (red) and no-mop zones (purple), cropped to
  the floor plan at 1.25 cm per pixel (4 pixels per 5 cm map cell, as
  python-roborock and Home Assistant draw it). The Qrevo S map renders at
  about 1460 x 1060; the dashboard scales it to the panel.
- Room edges are smoothed and rooms that touch are drawn with a thin gap
  between them. Walls are one grey; furniture specks of up to 15 cm inside a
  room are drawn as the room. Areas erased in the Roborock app are not drawn.
- Add `labels=names` to label rooms with `roborock.segment_names` (unnamed
  rooms show as `Room <id>`), or `labels=segments` to label them with ids.
- Add `path=false` to hide the last run's path (shown by default).
- The device's map is fetched at most every 5 seconds (cached).
- Map block numbers and pixel meanings follow `vacuum-map-parser-roborock`.

## Methods
- `ListDevices`: list available vacuums.
- `ListRooms`: list configured room names (preferred for room targeting).
- `CleanRoom`: clean a named room (preferred; accepts `room` like "kitchen" or "living room"; case-insensitive).
- `GetStatus`: read current status for a device.
- `StartClean`: start cleaning.
- `Pause`: pause cleaning.
- `Stop`: stop cleaning.
- `Dock`: return to dock.
- `Locate`: locate/beep the vacuum.
- `SetFanSpeed`: set fan speed by name or numeric code.
- `SetMopMode`: set mop mode by name or numeric code.
- `SetMopIntensity`: set mop intensity by name or numeric code.
- `CleanZone`: clean specific zones with optional repeats.
- `CleanSegment`: clean specific segments with optional repeats.
- `ListSegments`: fetch segment IDs from the current map snapshot (IDs only).
- `GoTo`: drive to coordinates.
- `SetDnd`: set DND schedule and enable/disable.
  - Time format: `HH:MM` (24-hour).
- `ResetConsumable`: reset a named consumable counter.

## CLI quick use
```
gohome-cli roborock status
gohome-cli roborock rooms
gohome-cli roborock clean kitchen
gohome-cli roborock mop kitchen
gohome-cli roborock vacuum kitchen
gohome-cli roborock smart kitchen
gohome-cli roborock map --labels names --path
```

## State
Metrics:
- `gohome_roborock_scrape_success`
- `gohome_roborock_battery_percent`
- `gohome_roborock_state`
- `gohome_roborock_error_code`
- `gohome_roborock_cleaning_area_square_meters`
- `gohome_roborock_cleaning_time_seconds`
- `gohome_roborock_total_cleaning_area_square_meters`
- `gohome_roborock_total_cleaning_time_seconds`
- `gohome_roborock_total_cleaning_count`
- `gohome_roborock_fan_speed`
- `gohome_roborock_mop_mode`
- `gohome_roborock_mop_intensity`
- `gohome_roborock_water_tank_attached`
- `gohome_roborock_mop_attached`
- `gohome_roborock_water_shortage`
- `gohome_roborock_charging`
- `gohome_roborock_last_clean_start_timestamp_seconds`
- `gohome_roborock_last_clean_end_timestamp_seconds`
- `gohome_roborock_last_clean_finished` (0 when the last run stopped early)

Areas are in square metres; the device reports mm² (`clean_area / 1e6`, as in
python-roborock). The cleaning area and time are the device's figures for
the current run, or the last one when docked. A run that stopped early
reports only what it covered.

## Errors
- Invalid or expired bootstrap credentials
- Device offline or unreachable over local TCP
- Command rejected by device
- Use `jq -n` to build JSON for gRPC calls to avoid quoting mistakes.

## Required Config
Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `roborock.bootstrap_file` (required; JSON with bootstrap credentials)
- `roborock.cloud_fallback` (optional; default false)
- `roborock.device_ip_overrides` (optional; map of device_id -> LAN IP to skip UDP discovery)
- `roborock.segment_names` (optional; map of segment_id -> room label)
  - Required when controlling a vacuum over Tailscale subnet routing (broadcast discovery won’t cross subnets)
- `roborock.default_profile` (optional; default fan/mop/repeat profile)
  - If unset, GoHome tries to derive defaults from the active Roborock schedule (`get_timer`).
  - If the schedule is missing/unreadable, falls back to `default_profile`.

Bootstrap JSON schema:
```json
{
  "schema_version": 1,
  "username": "user@example.com",
  "user_data": { "rruid": "...", "token": "...", "uid": "..." },
  "base_url": "https://user.roborock.com"
}
```

## Bootstrap Flow
Use the CLI to request a code and write the bootstrap JSON into the configured path:
```
gohome roborock bootstrap --email user@example.com
```
You can override the bootstrap file path with `--bootstrap-file`.
If you use agenix, write the bootstrap JSON to a temporary path first, then encrypt it into your secret store.

### Onboarding (local + remote)
1) Build GoHome with the Roborock plugin enabled (local dev or Nix build tag).
2) Run bootstrap to get the one-time email code and write JSON to a temp file:
```
gohome roborock bootstrap --email user@example.com --bootstrap-file /tmp/roborock-bootstrap.json
```
Roborock emails a short verification code to your account; paste it when prompted.
3) Encrypt the JSON into agenix (in your secrets repo):
```
agenix -e gohome-roborock-bootstrap.age
```
Paste the JSON contents from `/tmp/roborock-bootstrap.json` into the editor.
4) Wire secrets + config:
   - `nix/hosts/secrets.nix` adds `roborock-bootstrap` when the plugin is enabled.
   - `nix/hosts/gohome.nix` uses `config.age.secrets.roborock-bootstrap.path`.
5) Deploy: `scripts/deploy.sh gohome`

## Development
Use `devenv` for toolchain setup (Go + protoc plugins):
```
devenv shell
bash tools/generate.sh
```

## Connection reliability

Written by AI on 2026.11.06 by model gpt-6-astra.

Previously, dead local connections stayed cached indefinitely, and overlapping
requests could remove the wrong subscriber. Channels now use stable subscriber
IDs and are replaced after read, write, RPC timeout or keepalive failures.
Reads retry once; control commands are never retried automatically. Optional
cloud fallback reads the vacuum over MQTT, not cached discovery metadata.
Failed scrapes export only the failure gauge, never previous device readings.

The Qrevo S was verified using protocol `1.0`; L01 remains untested on hardware.
Dashboard map URLs must use `/gohome/roborock/map.png` through the reverse proxy;
`/roborock/map.png` is the direct service path on port 8080.

For the opt-in live test, set `ROBOROCK_TEST_BOOTSTRAP` to a private bootstrap
file and `ROBOROCK_TEST_HOST` to the vacuum address, then run:

```sh
devenv shell -- go test -race ./plugins/roborock -run TestLiveReadOnly -v -count=1
```

This requires exactly one vacuum in the account. It checks status, cleaning
history, connection recovery, concurrent reads, missing metrics on failure,
MQTT fallback and the HTTP map image. It sends no cleaning, docking or setting
commands; connection failures are injected into the test client's own sockets.

## Historical data integrity

Written by AI on 2026.11.06 by model gpt-6-astra.

