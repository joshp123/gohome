# Hikvision alarm history

Written by AI on 2026.11.09 by model claude opus 5.5.

The camera keeps its alarms in a ring-buffer log that holds weeks to months of
them. gohome exports `gohome_hikvision_last_alarm_start_timestamp_seconds` only
from when it starts watching that log. `backfill_alarms.py` imports the alarms
that the log still holds from before then, so the camera page's alarm list and
per-hour and per-day counts reach back as far as the log does.

## What it writes

Each alarm becomes one sample in the live series of its `kind`, with the same
labels. The sample's time is the alarm's start, and its value is that start
time in seconds, as the live gauge reads at that moment. Only alarms that started
before the series' first stored sample are imported. Existing samples are never
changed or deleted. The live gauge is sampled every scrape, so it holds only the
latest of two alarms that start within one scrape of each other. The import
holds one sample for every alarm.

## How to run it

It reads the alarms with `gohome-cli` over SSH on the host that runs the
hikvision plugin. `ListAlarms` is read-only and changes nothing on the camera.
It also reads the series from VictoriaMetrics. Keep `--directory` private and
outside the repo: it holds the camera's alarm times.

```
python3 -B plugins/hikvision/backfill_alarms.py --vm 'http://<hub>/vm' \
  --host <house host> --grpc '<house gohome gRPC address>' --directory <private folder>
```

Without `--import` it only reads. It saves the alarm list, a compressed export
of the whole series as it stands (the backup) and the proposed samples, and it
prints how many there are and their first and last times. Inspect them, then
run it again with `--import`. You can pass `--alarms <saved list>` to import
exactly the list you inspected. After the import, the script checks the
following and stops with an error if any check fails:
- every imported sample is stored with its time and value
- every sample in the backup is unchanged
- nothing else appeared before the first live sample

Every file a run writes has the run's time in its name, so a later run never
replaces an earlier backup or report. A second run proposes nothing.

To undo an import, delete the series with `/api/v1/admin/tsdb/delete_series`.
Then import the backup with `/api/v1/import` and compare a fresh export with it.
The delete has no time range, so the backup must hold the whole series.

## Checking the result

Count the log's alarms per local day and compare each day with the page's
"Alarms per day" query. Its days must match exactly. The list's rows must equal
the log's start times.
