# Growatt - Agent Context

## What This Is
Growatt provides cloud telemetry for solar/inverter systems via the ShinePhone app.

## Capabilities
- List plants
- Read plant energy overview
- Read inverter AC voltage/frequency, PV strings, temperatures and diagnostic registers

## Limits
- Cloud polling only (no LAN IP).
- Requires a valid Growatt OpenAPI token.
- Plant selection required if multiple plants exist.

## Methods
- `ListPlants`: returns available plants and IDs.
- `GetPlantStatus`: returns current power and energy totals.

## State
- Metrics: current power, today/month/year/total energy, last update timestamp, scrape success.

## Errors
- Invalid/expired token
- Plant not found
- API rate limits or outages

## Required Config

Canonical config file: `/etc/gohome/config.pbtxt` (textproto).

Fields:
- `growatt.token_file` (required; path to API token)
- `growatt.region` (optional; defaults to `other_regions`)
- `growatt.plant_id` (optional; required if multiple plants exist)

Token format (file contents):
```
<token>
```

## Historical data integrity

Written by AI on 2026.11.06 by model gpt-6-astra.



## Inverter API audit

Written by AI on 2026.11.08 by model gpt-6-astra.

The supported device is a MIN (OpenAPI type 7), with one populated AC phase,
two PV inputs and temperature channels 1, 2 and 5. `queryLastData` returns
all these readings in one response. `GetPlantStatus.inverter` exposes them
through typed `InverterQuantity` values with channel and source timestamp.
The same response supplies the metrics; no additional polling is required.
Missing, null and non-finite fields are absent. Electrical measurements expire
after 15 minutes and disappear after a failed read. Every exported inverter
sample carries the device's timestamp, interpreted using the plant timezone.

### Calls and throttling

Normal polling makes three requests every five minutes: `plant/data`,
`device/list`, and `v4/new-api/queryLastData`; plant discovery adds one request
at startup. This is 36 requests/hour, 12 per endpoint, unchanged by telemetry
collection. RPC requests are additional calls: do not use them as another poller.
`error_frequently_access` (10012) means stop, not retry in a tight loop. The
client returns errors immediately. The legacy energy backfill stops on an error
and spaces daily chunks at least five minutes apart by default. Never run it
alongside another history job or shorten that delay without proving the budget.

### Coverage

| What the API gives | Exported before | Exported now | Left out and why |
|---|---|---|---|
| Plant energy and inverter output power | Plant power and daily/monthly/yearly/lifetime energy | Same plus separately timestamped inverter AC energy | No computed efficiency or household consumption; derive in Grafana |
| AC voltage, current, apparent power, frequency, power factor | No | Populated phase and frequency/power factor | Unpopulated phases are API placeholders |
| PV voltage, current, power and energy | No | Both populated strings, total PV power and energy | Unpopulated PV inputs are placeholders |
| Temperatures | No | Channels 1, 2, 5 | Channels 3/4 are unpopulated; physical sensor locations unproven |
| DC bus voltage | No | Positive bus | Negative bus, DC and line-to-line aliases are unpopulated or redundant on this device |
| Fault words, warning/fault codes, operating status, derating, dry contact | No | Raw codes, including zero | Text says Unknown even when codes are zero; do not turn it into a fault |
| Output percentage, output limit, operating time, connection countdown, leakage current, insulation | No | Raw registers, explicit raw suffix where units/sentinels are unverified | No guessed scaling, fault thresholds or interpretation of 65530 insulation sentinel |
| Battery, EPS, load/export meter and external-inverter fields | No | No | No battery or meter fields are populated unless attached; response contains placeholders (including a spurious nonzero battery total) |
| Device detail and settings | No | No | Read-only probe succeeded. Hardware metadata and configuration, not new measured telemetry. Settings include voltage/frequency limits; these are not voltage readings. No extra periodic endpoint added |
| One-day inverter history | No | Offline conversion/import tool using the same decoder | API rejects dates outside three months (`DATE_WITHIN_3_MONTHS`); older surge hours cannot be recovered here |
| Plant daily/monthly/yearly energy history | Separate imported energy series | Existing importer retained | Full catch-up needs a bounded, paced run; not started concurrently with production |
| Serial/logger, calendar, debug, UI, transport and duplicate timestamp fields | No | Serial identifies inverter; source time retained | No unbounded debug labels, server-local calendar interpretation or UI metadata |

### Metric contract

All names below start with `gohome_growatt_inverter_`. All carry
`{plant_id,plant_name,serial,channel}`. Channel is empty for a scalar, the
one-based phase/PV/temperature index for channel readings, 0–7 for fault words,
or the vendor field name for diagnostic codes. Counters are reported as gauges
because they are vendor snapshots and may reset.

- `grid_frequency_hertz`: Grid frequency in Hz.
- `output_power_watts`: AC output power in watts.
- `pv_power_watts`: Total PV input power in watts.
- `power_factor`: Reported power factor.
- `today_energy_kwh`: Inverter daily AC energy in kWh.
- `total_energy_kwh`: Inverter lifetime AC energy in kWh.
- `pv_total_energy_kwh`: Lifetime PV input energy in kWh.
- `bus_voltage_volts`: DC bus voltage in volts.
- `operating_time_raw`: Raw timeTotal register; unit not verified.
- `output_limit_raw`: Raw opFullwatt output limit; scale not verified.
- `output_percent`: Reported output percentage.
- `connection_delay_raw`: Raw grid connection countdown; unit not verified.
- `derating_mode`: Reported derating mode code.
- `status`: Reported inverter status code.
- `operating_mode`: Reported operating mode code.
- `system_work_mode`: Reported system work mode code.
- `dry_contact_status`: Reported dry contact status code.
- `leakage_current_raw`: Raw grid leakage current register; scale not verified.
- `insulation_resistance_raw`: Raw PV insulation register; scale and sentinel values not verified.
- `grid_voltage_volts`: AC phase voltage in volts.
- `grid_current_amperes`: AC phase current in amperes.
- `phase_apparent_power_va`: Reported phase apparent power in VA.
- `string_voltage_volts`: PV input voltage in volts.
- `string_current_amperes`: PV input current in amperes.
- `string_power_watts`: PV input power in watts.
- `string_today_energy_kwh`: PV input today energy in kWh.
- `string_total_energy_kwh`: PV input total energy in kWh.
- `temperature_celsius`: Inverter temperature channel in degrees Celsius.
- `fault_word`: Raw system fault bitmask.
- `diagnostic_code`: Raw vendor diagnostic code.


### Evidence and history procedure

Live read-only probes of device list, latest data and device detail succeeded.
A captured, identity-redacted latest response is in `testdata/min.json`.
It reports grid voltage, frequency, PV input voltages and temperatures.
`TestCapturedMINResponse` runs that response through the
real collector and checks source timestamps, missing values, stale readings,
failed reads and exclusion of unpopulated hardware.

The historical endpoint returns one day of readings per request.
It rejects dates outside its three-month window.
A longer backfill needs a scheduled, bounded run at no more than one history
request per five minutes, stopping on the first rate-limit response. No
background job is left running. Confirm the provider's per-endpoint budget
before combining a long backfill with production polling.

To convert an already captured response without contacting Growatt:

```
go run -mod=readonly ./plugins/growatt/cmd/import-file \
  -input history.json -plant-id 42 -plant-name Solar \
  -serial INVERTER -timezone Europe/Amsterdam > history.prom
```

Use the plant's actual timezone for the requested date, not today's fixed UTC
offset across a daylight-saving transition. Before importing, back up the exact
`gohome_growatt_inverter_.*` series with VictoriaMetrics `/api/v1/export`, retain
the original response, and add the live target's `job`, `instance`, and `source`
labels. Import only the generated inverter series and compare every stored
sample's timestamp/value with the import file. Do not delete or rewrite other
Growatt series. The converter rejects truncated/paginated responses.

Post-deployment: check `/metrics/growatt` and `GetPlantStatus` during daylight
for the typed inverter readings, timestamps and absence after failed/stale reads.
No deployment is part of this branch.

References: [maintained MIN client](https://github.com/indykoning/PyPi_GrowattServer/blob/master/growattServer/open_api_v1/devices/min.py),
[v4 client and endpoint documentation](https://github.com/timohencken/GrowattPublicApiPy/blob/main/growatt_public_api/api_v4/api_v4.py),
[Growatt historical endpoint](https://www.showdoc.com.cn/2540838290984246/11292916022305414).


### Paced retained-history job

Written by AI on 2026.11.08 by model gpt-6-astra.

`tools/backfill.py` runs a bounded date range using the Go `cmd/import-file`
converter. It reads the hub's agenix token over SSH without copying it locally.
Required arguments identify the hub, token path, inverter, plant, VM endpoint,
converter and private output directory; `--help` lists them. No credentials or
personal identifiers belong in the script or in git.

The job allows one instance per output directory, saves each raw response,
backs up the target series globally and per day, imports only matching inverter
series, and verifies every source timestamp/value against a database export.
Pre-existing samples must remain unchanged. `state.json` records verified,
empty and refused days; restarting the same command resumes from that state.
A capture with an API error remains as evidence and must be examined before
resuming. Do not discard a rate-limit response and restart immediately.

History requests are at least six minutes apart and occur 90–220 seconds after
the live collector's last successful fetch, avoiding its five-minute request
burst. Every 30 seconds while waiting, and after each import, the job records
scrape success, last successful fetch age, inverter reading age and power.
Failed cloud reads or fetch age over 12 minutes stop the job. During 10:00–15:00
local time it also requires power and a source reading no older than 15 minutes.
Outside that winter daylight window it still records electrical freshness but
allows normal inverter sleep. The final day is followed by a further six-minute
observation window. Any API, import or verification error stops the job.

Dates are interpreted in the plant's named timezone so summer and winter source
times both align with meter readings. Refused retention-boundary days are
recorded and skipped; other API errors stop the run. Retention moves daily, so
request oldest dates first. A separate supervisor should inspect `state.json`
and the latest `events.jsonl` entries until status is `complete`; a stopped
process is not a completed backfill.
