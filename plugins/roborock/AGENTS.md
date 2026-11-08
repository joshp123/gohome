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


## Consumable wear

Written by AI on 2026.11.08 by model gpt-6-astra.

`GetStatus` includes typed, optional `consumables` fields. Each scrape reads
`get_consumable` through the same local connection/retry and optional MQTT
fallback as status. A failed read fails the scrape and exports no device values.
Missing fields remain absent; a reported zero remains zero. Counters are gauges
because maintenance resets them. They measure use since reset, not physical wear
or proof that a part has actually been replaced.

Both metric families have labels `{device_id, device_name, model, consumable}`:

- `gohome_roborock_consumable_used_seconds`: operating seconds, with consumable
  `main_brush`, `side_brush`, `filter`, `filter_element`, `sensors` or `mop_roller`.
- `gohome_roborock_consumable_used_cycles`: use counts, with consumable `strainer`,
  `dust_collection` or `cleaning_brush`. These are not elapsed seconds or days.

The Qrevo S reports the first five time fields, strainer and dust collection.
It does not report mop roller or cleaning brush counters. Its rotating mop pads
must not be represented by an invented mop-roller reading. A zero filter-element
counter alone does not establish that this model has a replaceable element.

### Remaining life in Grafana

The reference intervals below are from
[python-roborock const.py](https://github.com/Python-roborock/python-roborock/blob/16e7970ad3e255a03571aa2dd59a412b2c622d8c/roborock/const.py);
its [Consumable implementation](https://github.com/Python-roborock/python-roborock/blob/16e7970ad3e255a03571aa2dd59a412b2c622d8c/roborock/data/v1/v1_containers.py)
subtracts the corresponding device counters. These are the library's Roborock
maintenance intervals, not values returned by `get_consumable`. The exact Qrevo S
app screen and its percentage rounding have not been independently verified.

| Consumable | Interval | Unit |
| --- | ---: | --- |
| Main brush | 1,080,000 (300 h) | seconds |
| Side brush | 720,000 (200 h) | seconds |
| Filter | 540,000 (150 h) | seconds |
| Sensors, cleaning interval | 108,000 (30 h) | seconds |
| Mop roller, when reported | 1,080,000 (300 h) | seconds |
| Strainer | 150 | cycles |
| Dust collection | 90 | cycles |
| Cleaning brush, when reported | 300 | cycles |

No sourced lifetime is available for `filter_element`; show only its raw value.
The [protocol reference](https://github.com/marcelrv/XiaomiRobotVacuumProtocol/blob/master/consumable.md)
documents that field in seconds.

Grafana can calculate remaining percent as
`clamp_max(clamp_min(100 * (1 - used / interval), 0), 100)` for each component,
using the matching unit. Do not calculate or export these percentages in GoHome.
Do not fill absent parts with 100% or zero.

For short-run investigation, read `get_clean_summary` and `get_clean_record`
for its record IDs. Record `error` describes that run; current `get_status` error
is not historical evidence. `finish_reason=21` means manual interruption in
[python-roborock's reason mapping](https://github.com/Python-roborock/python-roborock/blob/16e7970ad3e255a03571aa2dd59a412b2c622d8c/roborock/data/v1/v1_code_mappings.py).
It does not identify the person, app or automation responsible.


## Read API coverage

Written by AI on 2026.11.08 by model gpt-6-astra.

| What the API gives | Exported before the audit | Exported now | Left out and why |
| --- | --- | --- | --- |
| Status, battery, cleaning area/time, suction and mop modes | Metrics and GetStatus | Retained | Nothing |
| Consumable time and use counters | Consumable metrics and typed GetStatus | Retained | Unsupported parts remain absent |
| Dock faults, charging/drying/washing, progress, avoidance, map and lock state | Mostly discarded | Optional typed GetStatus.readings and individual gauges below | Nothing with established meaning |
| Dock/robot/common sensor bitfields | Discarded | Raw bitfield gauges | No invented interpretation of reserved bits |
| Lifetime dust collection count | Discarded | total_dust_collections gauge and GetStatus | Nothing |
| Wi-Fi signal | Discarded | wifi_signal_dbm | SSID, IP, MAC and BSSID: network identities, not house readings |
| Dust collection, washing, drying and carpet settings | Discarded | Typed GetStatus.settings and nine gauges | Motor currents, cliff thresholds and heater tuning: firmware configuration |
| Latest cleaning record | Start/end and finished flag | All 17 observed fields; typed last_record; timestamped run metrics | Nothing |
| Retained cleaning history | Latest record only | GetCleaningHistory returns every retained record | Runs no longer retained cannot be reconstructed from lifetime totals |
| Current map and room geometry | PNG and ListSegments | Retained; GetMapDetails also returns the complete raw RR map | No raw map blocks are discarded by GetMapDetails |
| Room IDs, names and types | Configured names only | GetMapDetails.rooms joins device mapping to cloud names | No per-room duration or cleaned-area records were returned by the V1 record API |
| Obstacle markers | Discarded | GetMapDetails.obstacles: raw mm coordinates, type, ignored flag, photo ID when present | Confidence score not derived from undocumented numerator/denominator; raw map retains both |
| Historical map images | Not exposed | map.png?record_id=<record begin timestamp> | No map switch or restore is used |
| Saved map names, IDs, dates and backups | Not exposed | ListSavedMaps | Does not load a floor: that changes device state |
| Obstacle photos | Not exposed | No photos available on the tested Qrevo S | Structured-light model; observed marker has an empty photo ID. No dummy photo endpoint |
| Per-room cleaning sequence and custom modes | Not exposed | No readings available: both returned empty lists | Room configuration is not a per-room cleaning record |
| DND/charging timers, schedules, voice, LEDs, locale | Some control methods/default-profile reads | Status flags retained | UI and schedule configuration, not sensor history; no settings changed |
| Cleaning estimate internals | Not exposed | Not promoted to measurements | Idle response contains zero estimates and undocumented rate units; not measured cleaning data |
| msg_seq, msg_ver, lab/debug flags, feature masks, distance_off, kct, subdivision_sets, camera_status | Discarded | Not promoted to measurements | Protocol bookkeeping, capabilities or undocumented firmware configuration; camera_status alone does not prove camera hardware |
| get_dock_info, get_mop_motor_status, get_wash_towel_params | Not used | Not polled | This model returns unknown_method |
| Dynamic map diff/data | Not used | Full RR map retained instead | Incremental transport optimisation; no extra house measurement |

Sources: [Roborock's Qrevo S hardware description](https://ae.roborock.com/pages/roborock-qrevo-s),
[python-roborock V1 records and status](https://github.com/Python-roborock/python-roborock/blob/16e7970ad3e255a03571aa2dd59a412b2c622d8c/roborock/data/v1/v1_containers.py),
[Home Assistant's Roborock sensors](https://github.com/home-assistant/core/blob/dev/homeassistant/components/roborock/sensor.py)
(`rdt` is seconds), and
[ioBroker's V1 implementation](https://github.com/copystring/ioBroker.roborock/tree/main/src/lib/features/vacuum)
(`get_clean_record_map` takes `{start_time: <begin>}` over MQTT).
Obstacle records follow [vacuum-map-parser-roborock](https://github.com/PiotrMachowski/Python-package-vacuum-map-parser-roborock/blob/master/src/vacuum_map_parser_roborock/map_data_parser.py).

### Additional gauges

All have `{device_id, device_name, model}` labels; front sensors also have `sensor_index`. Missing fields are absent.
Numeric codes and bitfields are raw, not percentages or inferred booleans.
The prefix in the tables below is `gohome_roborock_`.

| Metric suffix | Unit / meaning |
| --- | --- |
| `dock_error_code` | Dock fault code |
| `dock_status_bits` | Raw dock sensor bitfield |
| `robot_status_bits` | Raw robot status bitfield |
| `common_status_bits` | Raw common status bitfield |
| `charge_status_code` | Charging state code |
| `dry_status_code` | Mop drying state code |
| `dry_remaining_seconds` | Remaining drying time in seconds |
| `cleaning_progress_percent` | Device cleaning progress percentage |
| `obstacles_avoided` | Obstacles avoided during this run |
| `dust_collection_status_code` | Dust collection state code |
| `auto_dust_collection_enabled` | Automatic dust collection enabled |
| `wash_phase_code` | Mop washing phase code |
| `wash_ready` | Mop washing readiness code |
| `wash_status_code` | Mop washing state code |
| `return_type_code` | Reason for returning to dock, device code |
| `cleaning_mode_code` | Cleaning operation code |
| `returning` | Returning to dock |
| `warming_up` | Warming up |
| `locating` | Locating on the map |
| `exploring` | Exploring |
| `dnd_enabled` | Do not disturb enabled |
| `child_lock_enabled` | Child lock enabled |
| `map_present` | Map present |
| `map_status_code` | Raw map state and map ID bitfield |
| `unsaved_map` | Unsaved map flag |
| `unsaved_map_reason_code` | Unsaved map reason code |
| `collision_avoidance_enabled` | Less collision mode enabled |
| `mop_forbidden_enabled` | Mop forbidden flag |
| `replenish_mode_code` | Water replenishment mode code |
| `corner_clean_mode_code` | Corner cleaning mode code |
| `cleaning_repeats` | Requested cleaning repeat count |
| `bumper_status_code{sensor_index}` | Raw front sensor codes; extra zero-based array index label |
| `total_dust_collections` | Lifetime dust collection count |
| `wifi_signal_dbm` | Wi-Fi received signal strength, dBm |
| `dust_collection_mode_code` | Raw configured emptying mode |
| `mop_wash_mode_code` | Raw configured mop washing mode |
| `smart_wash_enabled` | 0/1 smart mop washing setting |
| `mop_wash_interval_seconds` | Configured interval between washes, seconds |
| `dryer_enabled` | 0/1 dryer setting |
| `dryer_duration_seconds` | Configured drying duration, seconds |
| `carpet_boost_enabled` | 0/1 suction boost setting |
| `carpet_mode_code` | Raw carpet cleaning mode |

### Cleaning run history

`GetCleaningHistory` reads the record IDs from `get_clean_summary`, then each
record using `get_clean_record`. It returns individual records oldest first,
without combining partial records or inventing per-room totals. On the tested
Qrevo S the summary retains 20 records. The complete lifetime run count is much
larger; it does not imply that older individual records can still be fetched.

The `gohome_roborock_cleaning_run_` gauges use the device's **run end time** as
the Prometheus sample timestamp. They are sparse events, not current readings;
query with range functions or a sufficiently small step when showing history.
The latest completed record is emitted on each successful scrape with that same
original timestamp. The device's `complete=0` means interrupted, not a missing
end timestamp. Failed scrapes export none of these samples.

| Run metric suffix | Unit / meaning |
| --- | --- |
| `start_timestamp_seconds` | Start Unix timestamp |
| `end_timestamp_seconds` | End Unix timestamp |
| `duration_seconds` | Active cleaning seconds |
| `area_square_meters` | Cleaned area in square metres |
| `reported_cleaned_area_square_meters` | Device cleaned_area in square metres |
| `error_code` | Run error code |
| `finished` | Whether the run finished |
| `finish_reason_code` | Run finish reason code |
| `start_type_code` | Run start type code |
| `clean_type_code` | Run cleaning type code |
| `dust_collection_status_code` | Run dust collection state |
| `obstacles_avoided` | Number of obstacles avoided |
| `mop_washes` | Number of mop washes |
| `map_id` | Map ID |
| `cleaning_repeats` | Cleaning repeat count |
| `dirty_replenish_code` | Device dirty replenishment code |
| `manual_replenish_code` | Device manual replenishment code |

`GetStatus.last_record` and `GetCleaningHistory` expose the same optional typed
fields. Area is converted from mm² to m²; `area` and `cleaned_area` are kept
separate. Duration is active cleaning time, not end minus start. A long paused
run can span hours while recording only minutes of cleaning.

History backfill uses only the dedicated `gohome_roborock_cleaning_run_*`
series, canonical scrape labels and device end timestamps. Before importing,
export those exact destination series and retain the source records privately.
Do not overwrite the sampled `last_clean_*` or lifetime counters: individual
records cannot reconstruct those continuous histories. Verify every imported
value and timestamp using VictoriaMetrics' export API.

### Read cost and proof

A status scrape makes 11 device reads: status, summary, latest record,
consumables and seven operational-setting requests (10 with no records).
At the configured 15-second scrape interval that is 44 local RPCs per minute,
without extra cloud discovery. Failed local reads retry once; optional MQTT
fallback applies to the same reads. No published vendor rate allowance was
found; this count is not a claimed cloud quota. Full history is on demand
(21 reads for 20 records); map/details are on demand, with the existing
five-second current-map cache. No unsupported dock/motor methods are polled.

`TestLiveAPIReadOnly` verifies raw status against emitted metrics, operational
settings, device timestamps, retained records, room and saved-map APIs, and the
historical PNG HTTP route. It is opt-in with `ROBOROCK_TEST_BOOTSTRAP` and
`ROBOROCK_TEST_HOST`. Optional `ROBOROCK_HISTORY_EXPORT` writes VictoriaMetrics
JSONL to a private local path; it never imports it. Optional `ROBOROCK_AUDIT_MAP`
saves the historical PNG. The test sends only reads.

### Scrape-time copies

Written by AI on 2026.11.08 by model claude opus 5.5.

The hub's scrape job for this source honours sample timestamps (`nix/victoriametrics.nix`), so each run record is stored at the run's end. A scrape
job that does not honour them stores a copy of the latest record in every `cleaning_run_*` family at every scrape instead. A right sample's time equals the run's end, which `cleaning_run_end_timestamp_seconds` also holds as its value; a copy's time does not.
`tools/recovery/drop_misdated_samples.py roborock` keeps the right samples and removes the copies stored after `--since`.

Run it from a gohome checkout once the hub runs a build whose scrape job honours timestamps; before that, new copies
keep arriving and `--apply` refuses to change anything. The first run only exports, backs up and reports; `--apply`
repeats the export, deletes the series, imports the kept samples, clears VictoriaMetrics' cache of range results and
checks that every kept sample is stored and none is at a wrong time. The backup directory is private and outside
git. To restore, import `full-at-delete.jsonl.gz` (gunzipped) to `/api/v1/import`.

```sh
python3 -I tools/recovery/drop_misdated_samples.py roborock --vm http://<hub>/vm --since <first copy, ISO 8601 with offset> --backup-dir <private dir>
# Check the kept count matches the samples imported and verified before, and nothing is removed from before --since. Then add --apply.
```
