# Daikin Onecta - Agent Context

## What This Is
Daikin Onecta controls cloud‑connected HVAC units.

## Capabilities
- List units
- Read device state
- Toggle power, change mode, set temperature

## Limits
- Requires OAuth refresh state at runtime.
- Refresh tokens rotate; state is mutable and mirrored to blob storage.

## Methods
- `ListUnits`: returns available units and IDs.
- `GetUnitState`: returns raw JSON payload.
- `SetOnOff`: toggles power.
- `SetOperationMode`: sets HVAC mode.
- `SetTemperature`: sets target temperature.

## State
- Metrics include temperature, setpoint, humidity, power/mode, and last update timestamps.

## Required Config

Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `daikin.bootstrap_file` (required; JSON with client_id/client_secret, optional refresh_token)
- `oauth.blob_endpoint` (required)
- `oauth.blob_bucket` (required)
- `oauth.blob_access_key_file` (required)
- `oauth.blob_secret_key_file` (required)
- `oauth.blob_prefix` (optional, default `gohome/oauth`)
- `oauth.blob_region` (optional)

Auth flow:
- Auth-code flow via `gohome oauth auth-code --provider daikin --redirect-url <url>`

State file (written by runner): `/var/lib/gohome/daikin-credentials.json`
```json
{
  "schema_version": 1,
  "client_id": "...",
  "client_secret": "...",
  "refresh_token": "...",
  "scope": "openid onecta:basic.integration"
}
```

## Historical data integrity

Written by AI on 2026.11.06 by model gpt-6-astra.


## Read-only API coverage

Written by AI on 2026.11.08 by model gpt-6-astra.

`GET /v1/gateway-devices` contains the unit's measurements, configured states,
management-point faults and rolling electricity history. Collection uses the
existing ten-minute cache: at most 144 ordinary polling requests/day against the
200/day limit. Metrics, unit inspection and electricity share that cache. The
energy exporter reads a saved response and makes no cloud or OAuth requests.
Only the running hub owns token refresh.

| API data for this unit | Exported before this audit | Exported now | Left out and why |
|---|---|---|---|
| Room/outdoor temperature, humidity, per-mode setpoints | Yes | Same raw readings | No additional sensors in this response |
| Power, operation mode, cloud connectivity | Yes | Same raw states | — |
| Climate error/warning/caution flags | Yes | Flags on every management point | — |
| Gateway and outdoor-unit faults, raw error codes | No | Flags and `error_code_info` | Codes are preserved verbatim, including empty and `00-` |
| Fan mode, fixed speed, vertical direction per operation mode | No | `fan_mode`, `fan_speed_level`, `fan_direction` | These are configured settings, not measured fan speed or proof that the fan runs |
| Powerful mode configured/active | No | `powerful_mode`, `powerful_mode_active` | — |
| Holiday active/enabled | Active only | Active and `holiday_mode_enabled` | — |
| Schedule mode, enabled state, selected schedule | No | `schedule_mode`, `schedule_enabled`, `schedule_selected` | — |
| Stored schedule actions and schedule names | Raw RPC only | Raw RPC | Configuration, not executed events; `nextAction` is empty on this unit |
| Gateway firmware/model and indoor software version | No | `firmware_info` | Update-support capability is static API metadata |
| Gateway timestamp | No | `device_timestamp_seconds` | This timestamps the response, not an independent timestamp for every sensor |
| Heating/cooling electricity history | Raw RPC only | Raw slots, interval timestamps, completed history | No power estimate, derived counter, sums or interpolated history |
| IP, MAC, SSID, serial number | Raw RPC only | Raw RPC | Private network/identity metadata, not house readings |
| Allowed values, ranges, steps, settable flags, management-point categories | Raw RPC only | Raw RPC | API schema and control capabilities, not measurements |
| Gas consumption, thermal output, horizontal swing, air purification, images | Absent | Absent | Not supplied by this unit; do not create empty or zero series |

The maintained [Onecta client](https://github.com/jwillemsen/daikin-onecta/blob/main/src/daikin_onecta/client.py)
also exposes `GET /v1/sites`. Its [site model](https://github.com/jwillemsen/daikin-onecta/blob/main/src/daikin_onecta/models.py)
is account membership, location and gateway IDs, not readings or history. It is
not polled. Its other management-point operations are writes and are not part
of this read-only audit. No separate historical-read endpoint appears in that
client. This is the verified client surface, not a claim that every possible
vendor endpoint has been inspected.

Daikin's [official climate-control documentation](https://developer.cloud.daikineurope.com/docs/84e709f1-9d33-47e1-a93c-7f5cb8b8f12b/supported_features/supported_features_climatecontrol)
requires a developer login; the documentation API returns HTTP 401. The live
response proves the array sizes, units and values. The maintained client's
`EnergyData.current_total` and the [integration's sensor implementation](https://github.com/jwillemsen/daikin_onecta/blob/master/custom_components/daikin_onecta/sensor.py)
confirm previous/current halves. Calendar mapping is implementation-based;
direct confirmation against the authenticated vendor documentation remains open.

### Metric contract

All names below start with `gohome_daikin_`. Unless stated otherwise, labels are
`unit_id`, `unit_name`, `embedded_id`. All are gauges; missing readings are absent.
Failed source reads export none of these device readings.

| Suffix | Extra labels | Unit and meaning |
|---|---|---|
| `device_timestamp_seconds` | — | Epoch seconds of the gateway response |
| `error_code_info` | `code` | 1 for the raw reported code |
| `firmware_info` | `firmware`, `software`, `model` | 1 for reported versions/model; unavailable fields are empty |
| `powerful_mode` | — | 1 on, 0 off, configured |
| `powerful_mode_active` | — | 1 active, 0 inactive |
| `holiday_mode_enabled` | — | 1 enabled, 0 disabled |
| `fan_mode` | `operation_mode`, `mode` | 1 for selected configured fan mode |
| `fan_speed_level` | `operation_mode`, `mode` | Raw fixed-speed level, 1–5 on this unit, not RPM or percent |
| `fan_direction` | `operation_mode`, `axis`, `direction` | 1 for configured direction |
| `schedule_mode` | `mode` | 1 for selected schedule mode |
| `schedule_enabled` | `mode` | 1 enabled, 0 disabled |
| `schedule_selected` | `mode`, `schedule` | 1 for selected schedule ID |
| `electricity_slot_kwh` | `operation_mode`, `resolution`, `slot` | Unmodified consumption per API bucket, including incomplete current buckets |
| `electricity_slot_start_timestamp_seconds` | same as slot kWh | Epoch seconds anchoring that bucket |
| `electricity_observed_timestamp_seconds` | **only** `unit_id`, `unit_name` | Gateway timestamp anchoring all arrays |
| `electricity_consumption_kwh` | `operation_mode`, `resolution` | Completed raw bucket, with sample timestamp at interval start |

`resolution` is `2h`, `day` or `month`; **never sum across resolutions**.
`slot` is the zero-based API array index. `d` holds yesterday/today (12 two-hour
slots each), `w` holds previous/current Monday-based weeks (7 days each), and
`m` holds previous/current calendar years (12 months each). Dates use the
response's gateway timezone and timestamp, never the collector machine's date.
Future slots and null values are omitted. Two-hour slots on a DST transition
day are omitted because the fixed 12-slot format cannot identify the repeated
or missing hour; daily/monthly totals remain available.

The completed-history metric exports the latest complete bucket at each
resolution after a two-hour publication allowance. Explicit source timestamps
keep publication delay out of the consumption timeline. Repeated scrapes carry
the same timestamp; they are not additional energy. A daily graph must read
one sample per calendar day with a suitable lookback, rather than rate() or
increase(). Older monthly totals cannot be split into invented daily usage.
Long outages need the backfill tool to recover the rolling window.

### Backfill

`GetUnitState` supplies the full rolling window without another OAuth client.
Save its `json` field as a raw gateway payload. From a development checkout:

```sh
devenv shell -- go run -mod=readonly ./plugins/daikin/cmd/daikin-energy-export \
  --input /path/to/gateway.json > /path/to/energy.jsonl
python3 plugins/daikin/backfill.py --input /path/to/energy.jsonl \
  --vm-url http://<hub>/vm \
  --backup-dir /path/to/private-backups
```

The importer backs up the exact target series and import payload before writing.
It imports only `gohome_daikin_electricity_consumption_kwh`, skips identical
stored samples, refuses conflicting values and never deletes data. It does not
read or refresh tokens. Keep the gateway JSON beside the backup. After importing,
export the same metric and compare every label, timestamp and value with the
input; HTTP success alone is not verification.


After deployment, check `/metrics/daikin` on the hub for the metric contract
above, and that new samples are stored at their buckets' start times.
No deployment is performed by the audit/export tools.

### Scrape-time copies

Written by AI on 2026.11.08 by model claude opus 5.5.

The hub's scrape job for this source honours sample timestamps (`nix/victoriametrics.nix`), so each completed bucket is stored at its period's start. A scrape
job that does not honour them stores a copy of the latest bucket of each mode and resolution at every scrape instead. A right sample sits on a whole hour (bucket starts are whole local hours, and most timezone offsets are whole hours); a copy never does.
`tools/recovery/drop_misdated_samples.py daikin` keeps the right samples and removes the copies stored after `--since`.

Run it from a gohome checkout once the hub runs a build whose scrape job honours timestamps; before that, new copies
keep arriving and `--apply` refuses to change anything. The first run only exports, backs up and reports; `--apply`
repeats the export, deletes the series, imports the kept samples, clears VictoriaMetrics' cache of range results and
checks that every kept sample is stored and none is at a wrong time. The backup directory is private and outside
git. To restore, import `full-at-delete.jsonl.gz` (gunzipped) to `/api/v1/import`.

```sh
python3 -I tools/recovery/drop_misdated_samples.py daikin --vm http://<hub>/vm --since <first copy, ISO 8601 with offset> --backup-dir <private dir>
# Check the kept count matches the samples imported and verified before, and nothing is removed from before --since. Then add --apply.
```
