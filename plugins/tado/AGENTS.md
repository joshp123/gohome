# Tado - Agent Context

## What This Is
Tado controls cloud‑connected thermostats and heating zones.


## Capabilities
- List zones
- Set target temperature per zone

## Limits
- Requires OAuth refresh state at runtime.
- Refresh tokens rotate; state is mutable and mirrored to blob storage.
- Temperature range and valid zone IDs depend on the Tado account.

## Methods
- `ListZones`: returns available zones and IDs.
- `SetTemperature`: sets target temperature for a zone.

## CLI quick use
```
gohome-cli tado zones
gohome-cli tado set <zone> 20
```

## State
- Metrics: inside temperature, humidity, target setpoint, heating power, power/override status, and last update timestamp per zone; outside temperature and solar intensity at home level.
- `gohome_tado_device_battery_low{zone_name,serial,device_type}`: 1 when Tado reports the device's battery low, 0 when normal. Tado gives no battery level, as in its app. Absent for devices without a battery, and when the read fails.

## Errors
- API auth failures
- Zone not found

## Required Config

Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `tado.bootstrap_file` (required; JSON with client_id/client_secret, optional refresh_token)
- `tado.home_id` (optional override; auto-discovered if omitted)
- `oauth.blob_endpoint` (required)
- `oauth.blob_bucket` (required)
- `oauth.blob_access_key_file` (required)
- `oauth.blob_secret_key_file` (required)
- `oauth.blob_prefix` (optional, default `gohome/oauth`)
- `oauth.blob_region` (optional)

Auth flow:
- Device flow via `gohome oauth device --provider tado`

State file (written by runner): `/var/lib/gohome/tado-token.json`
```json
{
  "schema_version": 1,
  "client_id": "...",
  "client_secret": "...",
  "refresh_token": "...",
  "scope": "offline_access"
}
```

## Historical data integrity

Written by AI on 2026.11.06 by model gpt-6-astra.


## API coverage audit and safe source reads

Written by AI on 2026.11.08 by model gpt-6-astra.


`GetSourceData` reads the unmodified vendor response through the running hub's
OAuth manager. Its resource enum allows ZONES, ZONE_STATES, WEATHER, HOME_STATE,
DEVICES and DAY_REPORT. DAY_REPORT requires a positive zone ID and YYYY-MM-DD
date. Response JSON is protobuf bytes; the response also carries `ratelimit`
and `ratelimit-policy` headers. It cannot call arbitrary URLs or write settings.
Each call costs one source request after initial home discovery. No periodic
requests are added. Use stdin with `gohome-cli call` for request JSON.

| What the API gives | Exported before | Exported now | Left out and why |
|---|---|---|---|
| Temperature, humidity, heating demand and timestamps | Values, temperature timestamp | Values and all sensor timestamps | No separate valve-position measurement in the checked schema |
| Zone HOME/AWAY, connection, window detection, override/termination, geolocation override, preparation, offline schedule | Selected power/override booleans | Metrics below | HOME/AWAY is a reported mode, not physical occupancy |
| Device connection, mounting/errors, child lock, pairing | Battery only | Battery plus metrics below | Static capability, orientation and firmware strings are not measurements |
| Weather condition and timestamps | Temperature and solar intensity | Condition and all source timestamps too | None from the checked response schema |
| Daily zone reports | Selected fields, categorical conversions | Separate history series; categories and durations retained | No interpolation into live samples |
| Home account presence/away reason | Not collected | Zone HOME/AWAY retained | Separate `/state` endpoint not polled: account quota is unverified |
| Display preferences, future schedule changes, capabilities, account data | Not collected | Not collected | Configuration or personal data, not current observations |

The cloud audit is incomplete until this reader is deployed. The hub state file
contains only a rotating refresh token; its access token lives in the running
process. A second OAuth manager would compete with that process. Do not run a
local cloud collector or refresh tokens to obtain audit data.

The collector currently makes three GET requests per collection (zones, combined
zone states, weather), plus one home discovery per process. At a 15-second
collection interval this is 17,280 requests/day. The
[vendor quota documentation](https://help.tado.com/en/articles/12165739-limitation-for-rest-api-usage)
lists 100/day or 20,000/day according to subscription. Read this account's actual
headers before making audit/history calls; no quota has been assumed verified.

## Additional metrics

Written by AI on 2026.11.08 by model gpt-6-astra.

All use only `/zones`, `/zoneStates` and `/weather`. Devices are nested in the
zone-list response; the collector does not make a separate `/devices` request.
Missing fields are absent. Explicit null window/override states mean inactive.
Power ON does not imply heating activity when heating demand is absent.

Schema evidence: [PyTado's device and zone models](https://github.com/wmalgadey/PyTado/tree/master/PyTado/models/pre_line_x)
and its checked-in response fixtures. Live value comparison remains a post-deploy
check; no new cloud measurements are claimed verified from these fixtures.

| Metric (`gohome_tado_` prefix) | Meaning | Labels |
|---|---|---|
| `zone_connection_state` | Reported zone connection state | zone_id, zone_name, state |
| `zone_presence` | Tado HOME/AWAY mode, not a physical occupancy measurement | zone_id, zone_name, mode |
| `zone_overlay_type` | Reported override type; NONE for explicit null | zone_id, zone_name, type |
| `zone_preparation` | Reported preparation state | zone_id, zone_name, state |
| `zone_geolocation_override` | Geolocation override enabled (0/1) | zone_id, zone_name |
| `zone_geolocation_override_disable_timestamp_seconds` | Geolocation override disable time (Unix seconds) | zone_id, zone_name |
| `zone_running_offline_schedule` | Running the offline schedule (0/1) | zone_id, zone_name |
| `zone_open_window_detected` | Tado detects an open window (0/1); not a contact sensor | zone_id, zone_name |
| `zone_open_window_duration_seconds` | Reported suspension duration (seconds) | zone_id, zone_name |
| `zone_open_window_remaining_seconds` | Remaining open-window suspension (seconds) | zone_id, zone_name |
| `zone_open_window_expiry_timestamp_seconds` | Open-window suspension expiry (Unix seconds) | zone_id, zone_name |
| `zone_open_window_detected_timestamp_seconds` | Open-window detection time (Unix seconds) | zone_id, zone_name |
| `zone_overlay_termination` | Reported override termination type | zone_id, zone_name, type |
| `zone_overlay_duration_seconds` | Reported override duration (seconds) | zone_id, zone_name |
| `zone_overlay_remaining_seconds` | Remaining override duration (seconds) | zone_id, zone_name |
| `zone_overlay_expiry_timestamp_seconds` | Projected override expiry (Unix seconds) | zone_id, zone_name |
| `humidity_last_updated_timestamp_seconds` | Humidity observation time (Unix seconds) | zone_id, zone_name |
| `heating_power_last_updated_timestamp_seconds` | Heating demand observation time (Unix seconds) | zone_id, zone_name |
| `device_connected` | Device connection state (0/1) | zone_name, serial, device_type |
| `device_connection_timestamp_seconds` | Device connection state observation time (Unix seconds) | zone_name, serial, device_type |
| `device_mounting_state` | Device mounting state | zone_name, serial, device_type, state |
| `device_mounting_state_with_error` | Device mounting/error state | zone_name, serial, device_type, state |
| `device_mounting_timestamp_seconds` | Device mounting state observation time (Unix seconds) | zone_name, serial, device_type |
| `device_child_lock_enabled` | Device child lock enabled (0/1) | zone_name, serial, device_type |
| `device_pairing_mode` | Device pairing mode (0/1) | zone_name, serial, device_type |
| `weather_state` | Reported weather condition | state |
| `weather_last_updated_timestamp_seconds` | Weather condition observation time (Unix seconds) | none |
| `outside_temperature_last_updated_timestamp_seconds` | Outdoor temperature observation time (Unix seconds) | none |
| `solar_intensity_last_updated_timestamp_seconds` | Solar intensity observation time (Unix seconds) | none |

## History recovery

`recover_history.py` runs through the hub RPC, never through a second OAuth
manager. Supply an explicit date window and private output directory; end date
is exclusive. The first run prepares an import without writing VictoriaMetrics:

```
python3 -B plugins/tado/recover_history.py --host root@hub.example --vm 'http://<hub>/vm' --start 2026-11-07 --end 2026-11-08 --directory /private/history/tado
```

It saves a gzip backup plus SHA-256, daily source reports, `missing.jsonl` and
`verification.json`. Repeat the same command with `--import` to write and verify.
Existing timestamps, including conflicting values, are left untouched and counted.
The actual quota header must leave enough calls for the collector until reset
plus 100 reserved requests. Cached reports can be reused after an interrupted run.
Start with one day to establish the response and retention; expand only within
that verified quota budget.

History uses `gohome_tado_history_*`, separate from live samples. Day-report
points are not assumed to be instantaneous sensor observations. Heating demand,
mode and weather intervals retain categorical labels and the source interval's
duration in seconds at its start timestamp. Sunny is boolean, not solar intensity.
No null values or missing timestamps become zero. Sub-1% humidity stays sub-1%.
The Go backfill converter follows the same principles for its supported fields;
use the RPC recovery script for backups, quota checks and safe resumption.

Post-deploy checks:
1. Read ZONES, ZONE_STATES and WEATHER through GetSourceData. Record quota headers;
   compare every returned observational field against the table, including fields
   absent in the fixtures. No settings writes or OAuth refresh outside the hub.
2. Compare `/metrics` to those responses: link, presence, window/override nulls,
   battery/connection/mounting states and timestamps. Check failed collections
   retain only health metadata, not device values.
3. Prepare one complete day's history for all zones. Inspect source point and
   interval timestamps, category names, humidity units and the candidate series.
4. Expand to the available history within quota; review the backup checksum and
   missing-sample counts, then import. Require matching post-import values and
   counts; rerunning must prepare zero already-imported samples.
