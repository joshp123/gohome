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
- Fan/mop numeric codes are device-specific; prefer named values unless you know the codes

## Live Map (v2)
- Live map requires MQTT access via Roborock cloud (map RPC channel).
- HTTP endpoint: `http://<gohome-host>:8080/roborock/map.png?device_name=<device>`
- Add `labels=segments` to overlay segment IDs.
- Add `labels=names` to overlay configured room names.
- Add `path=true` to overlay the cleaning trace (default on).
- Map refresh interval is 5 seconds (cached).

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
