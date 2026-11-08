# Weheat history recovery

Written by AI on 2026.11.08 by model gpt-6-astra.

Read AGENTS.md's historical-data and thermostat-sentinel sections first.

The API audit is awaiting deployment of the complete `GetLogViews` reader.
The typed dependency omits DHW target/control and cooling diagnostic fields
from hourly history. Do not backfill through that incomplete reader.

After deployment, use the hub's RPCs with its existing OAuth manager. Never
start a separate token refresher. Request seven-day windows, at Hour interval,
and inspect the first and last returned buckets to establish available history.
The API can include the end bucket: deduplicate by metric, labels and timestamp.

Before importing, export all affected `gohome_weheat_history_log_*` and
`gohome_weheat_history_energy_*` series from VictoriaMetrics to a private backup,
verify its gzip integrity and record its checksum. Retain source responses too.
Compare timestamps against stored data and import only missing observations;
never delete live series or overwrite the previous sentinel repair.

Use the source `timeBucket` as the sample timestamp and preserve `interval`.
Never put hourly averages into live `log_*` series. For tRoom and tRoomTarget,
exclude average/minimum/maximum together whenever the bucket's minimum is -1.
Keep real -1 outdoor temperatures. Missing and null fields remain absent.
Verify imported counts and sample values against the retained API responses.

## Executable recovery and deployment checks

Written by AI on 2026.11.08 by model gpt-6-astra.

`recover_history.py` prepares missing hourly observations from complete hub
responses. It archives each seven-day response, saves the affected history's
compressed backup and checksum, preserves existing samples (including conflicts),
and writes `missing.jsonl` and `verification.json`. It uses the same metric-name
conversion and label set as the existing history, so resumed work does not create
a second copy under different names. It skips each thermostat field's entire
average/min/max triplet when that field's minimum is -1.

```
python3 -B plugins/weheat/recover_history.py --host root@hub.example --vm 'http://<hub>/vm' --start <YYYY-MM-DD> --end <YYYY-MM-DD> --directory /private/history/weheat
```

This only reads the hub and database. Add `--import` after inspecting the private
source archives and proposed samples. The script then verifies every imported
value and timestamp. Missing JSON fields remain missing. A resumed import adds
only observations not already present. Source archive filenames include both ends of the request window.

Exact checks after the combined deployment:
1. GetEnergyTotals must retain all returned `totalEInIU*` values. Compare each
   with its `energy_total_ein_iu_*_kwh` metric, including genuine zero values.
2. GetLogViews for one completed day must retain vendor DHW target/control,
   cooling conditions/reasons/backoff and last-cooling fields when supplied.
   Compare its keys with the vendor debugger schema; absent fields are not zero.
3. Compare device state/status metrics with ListHeatPumps and last-cooling epoch
   seconds with GetLatestLog when the latter returns a timestamp.
4. Prepare the documented recovery interval, inspect missing counts and backup
   integrity, then import and verify. Probe earlier explicit windows to establish
   the provider's retention and extend only where actual buckets exist. Keep
   recovered hourly aggregates separate from instantaneous live readings.
5. Confirm source-value equality after import, unchanged existing observations,
   and a second run with zero already-imported samples. Retain counts and hashes
   in the private verification manifest; do not commit device identifiers.
