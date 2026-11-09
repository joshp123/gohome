"""Import the alarms the camera's log still holds from before gohome watched it, as gohome_hikvision_last_alarm_start_timestamp_seconds.

Written by AI on 2026.11.09 by model claude opus 5.5. See RECOVERY.md.

Each alarm becomes one sample at its start time whose value is that start time, in the live series of its kind (same
labels), and only before that series' first stored sample. Existing samples are never changed. Without --import it
only reads, backs up and writes the proposed samples; with --import it imports them and checks every stored sample.
"""
import argparse
import datetime as dt
import gzip
import hashlib
import json
import pathlib
import subprocess
import time
import urllib.parse
import urllib.request

METRIC = "gohome_hikvision_last_alarm_start_timestamp_seconds"


def http(url, data=None):
    with urllib.request.urlopen(urllib.request.Request(url, data), timeout=120) as response:
        return response.read()


def export(vm):
    """Every stored sample of the metric: {labels: {timestamp_ms: value}}."""
    raw = http(vm + "/api/v1/export?" + urllib.parse.urlencode({"match[]": METRIC}))
    series = {}
    for line in raw.decode().splitlines():
        row = json.loads(line)
        labels = tuple(sorted(row["metric"].items()))
        series.setdefault(labels, {}).update(zip(row["timestamps"], row["values"]))
    return raw, series


def read_alarms(host, grpc_addr):
    """Every alarm the camera's log holds, as gohome's ListAlarms returns them (one JSON object per line)."""
    command = f"GOHOME_GRPC_ADDR={grpc_addr} gohome-cli --json hikvision alarms 87600h"
    return subprocess.run(["ssh", host, command], check=True, capture_output=True).stdout


def kind_label(kind):
    return kind.removeprefix("EVENT_KIND_").lower()   # as the plugin's metrics.go names it


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--vm", required=True, help="VictoriaMetrics base URL, e.g. http://<hub>/vm")
    parser.add_argument("--host", help="SSH host running the gohome with the hikvision plugin")
    parser.add_argument("--grpc", help="that gohome's gRPC address, as gohome-cli takes it")
    parser.add_argument("--alarms", help="a saved ListAlarms JSON-lines file to use instead of reading the camera again")
    parser.add_argument("--directory", required=True, help="private folder for the backup, the source and the report")
    parser.add_argument("--import", dest="import_data", action="store_true")
    args = parser.parse_args()
    out = pathlib.Path(args.directory)
    out.mkdir(parents=True, exist_ok=True)
    # Every file a run writes has the run's time in its name, so a rerun never replaces the backup or report of an import.
    run = f"{dt.datetime.now(dt.timezone.utc):%Y%m%dT%H%M%SZ}"

    source = pathlib.Path(args.alarms).read_bytes() if args.alarms else read_alarms(args.host, args.grpc)
    (out / f"list-alarms-{run}.jsonl").write_bytes(source)
    raw, before = export(args.vm)
    backup = out / f"before-export-{run}.jsonl.gz"
    backup.write_bytes(gzip.compress(raw))
    gzip.decompress(backup.read_bytes())
    sums = {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in (backup, out / f"list-alarms-{run}.jsonl")}

    live = {dict(labels)["kind"]: labels for labels in before}
    proposed, skipped = {}, {"stored_already": 0, "after_first_live_sample": 0}
    alarms = [json.loads(line) for line in source.decode().splitlines() if line.strip()]
    for alarm in alarms:
        labels = live.get(kind_label(alarm["kind"]))
        if labels is None:
            raise SystemExit(f"no live {METRIC} series for kind {kind_label(alarm['kind'])}: its labels are unknown")
        start = dt.datetime.fromisoformat(alarm["startedAt"].replace("Z", "+00:00")).timestamp()
        ms = round(start * 1000)
        if ms in before[labels]:
            skipped["stored_already"] += 1
        elif ms >= min(before[labels]):
            skipped["after_first_live_sample"] += 1
        else:
            proposed.setdefault(labels, {})[ms] = start
    lines = [json.dumps({"metric": dict(labels), "values": [samples[ms] for ms in sorted(samples)], "timestamps": sorted(samples)})
             for labels, samples in proposed.items()]
    (out / f"proposed-{run}.jsonl").write_text("".join(line + "\n" for line in lines))
    times = sorted(ms for samples in proposed.values() for ms in samples)
    report = {"alarms_in_log": len(alarms), "proposed_samples": len(times), "skipped": skipped,
              "first": dt.datetime.fromtimestamp(times[0] / 1000, dt.timezone.utc).isoformat() if times else None,
              "last": dt.datetime.fromtimestamp(times[-1] / 1000, dt.timezone.utc).isoformat() if times else None,
              "sha256": sums, "imported": False}

    if args.import_data and lines:
        http(args.vm + "/api/v1/import", "".join(line + "\n" for line in lines).encode())
        for _ in range(30):   # imported samples become readable within seconds
            _, after = export(args.vm)
            if all(set(proposed[labels]) <= set(after.get(labels, {})) for labels in proposed):
                break
            time.sleep(1)
        wrong = [(labels, ms) for labels, samples in proposed.items() for ms, value in samples.items()
                 if after.get(labels, {}).get(ms) != value]
        changed = [(labels, ms) for labels, samples in before.items() for ms, value in samples.items()
                   if after.get(labels, {}).get(ms) != value]
        # Live scrapes keep adding samples after the first one; nothing else may appear before it.
        unexpected = [(labels, ms) for labels in after for ms in after[labels]
                      if labels not in before or (ms < min(before[labels]) and ms not in proposed.get(labels, {}))]
        report.update(imported=True, imported_wrong=len(wrong), existing_changed=len(changed), unexpected_samples=len(unexpected))
        if wrong or changed or unexpected:
            (out / f"report-{run}.json").write_text(json.dumps(report, indent=1))
            raise SystemExit("verification failed; see the run's report and restore from the before-export backup if needed")
    (out / f"report-{run}.json").write_text(json.dumps(report, indent=1))
    print(json.dumps(report, indent=1))


if __name__ == "__main__":
    main()
