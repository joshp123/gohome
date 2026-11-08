"""Read hub RPCs, archive responses, and import only missing source observations."""
import argparse
import datetime as dt
import gzip
import hashlib
import json
import pathlib
import re
import subprocess
import time
import urllib.parse
import urllib.request


def timestamp(value):
    return int(dt.datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp() * 1000)


def rpc(host, service, method, data):
    command = ["ssh", host, "gohome-cli call " + service + "/" + method]
    return json.loads(subprocess.check_output(command, input=json.dumps(data).encode()))


def http(url, data):
    return urllib.request.urlopen(urllib.request.Request(url, data=data), timeout=120).read()


def series_key(metric):
    return tuple(sorted(metric.items()))


def export(vm, selector):
    form = urllib.parse.urlencode({"match[]": selector}).encode()
    return http(vm + "/api/v1/export", form)


def observations(payload):
    result = {}
    for line in payload.splitlines():
        row = json.loads(line)
        key = series_key(row["metric"])
        for stamp, value in zip(row["timestamps"], row["values"]):
            result[(key, stamp)] = value
    return result


def recover(source, fetch):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", required=True)
    parser.add_argument("--vm", required=True, help="VictoriaMetrics base URL, e.g. http://<hub>/vm")
    parser.add_argument("--start", required=True, type=dt.date.fromisoformat)
    parser.add_argument("--end", required=True, type=dt.date.fromisoformat, help="exclusive date")
    parser.add_argument("--directory", required=True, type=pathlib.Path)
    parser.add_argument("--import", dest="import_data", action="store_true")
    args = parser.parse_args()
    if args.end <= args.start:
        parser.error("end must be after start")
    args.directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    # Never replace a backup on a resumed run.
    selector = '{__name__=~"gohome_' + source + '_history_.*"}'
    backup = args.directory / "before.jsonl.gz"
    if not backup.exists():
        with gzip.open(backup, "wb") as target:
            target.write(export(args.vm, selector))
    with gzip.open(backup, "rb") as source_file:
        original = source_file.read()
    observations(original)
    (args.directory / "backup.sha256").write_text(hashlib.sha256(backup.read_bytes()).hexdigest() + "\n")
    # Re-read current storage too: never overwrite another recovery's work.
    existing = observations(export(args.vm, selector))
    proposed = {}
    conflicts = set()
    for metric, stamp, value in fetch(args):
        if not (timestamp(args.start.isoformat()+"T00:00:00Z") <= stamp < timestamp(args.end.isoformat()+"T00:00:00Z")):
            continue
        identity = (series_key(metric), stamp)
        if identity in proposed and proposed[identity] != value:
            raise ValueError("source reports disagree at one timestamp; inspect private archive")
        if identity in existing:
            if existing[identity] != value:
                conflicts.add(identity)
            continue
        proposed[identity] = value
    rows = [dict(metric=dict(key), timestamps=[stamp], values=[value]) for (key, stamp), value in sorted(proposed.items())]
    payload = b"".join(json.dumps(row, allow_nan=False).encode()+b"\n" for row in rows)
    (args.directory / "missing.jsonl").write_bytes(payload)
    report = {"prepared_samples": len(rows), "series": len({key for key, _ in proposed}), "existing_conflicts_left_unchanged": len(conflicts), "imported_samples": 0}
    if args.import_data and payload:
        http(args.vm + "/api/v1/import", payload)
        for attempt in range(30):
            actual = observations(export(args.vm, selector))
            if all(actual.get(identity) == value for identity, value in proposed.items()):
                break
            time.sleep(1)
        if any(actual.get(identity) != value for identity, value in proposed.items()):
            raise ValueError("import verification failed; keep source archive and backup")
        report["imported_samples"] = len(rows)
    (args.directory / "verification.json").write_text(json.dumps(report, indent=2)+"\n")
    print(json.dumps(report))


def snake(name):
    # Match the collector and existing history series, including EIN/IU acronyms.
    out = ""
    previous_lower = False
    for index, char in enumerate(name):
        if char == "_":
            out += char
            previous_lower = False
            continue
        upper = char.isupper()
        if index and upper and previous_lower:
            out += "_"
        out += char.lower()
        previous_lower = not upper
    return out


def fetch_history(args):
    service = "gohome.plugins.weheat.v1.WeheatService"
    pumps = rpc(args.host, service, "ListHeatPumps", {})["heatPumps"]
    for pump in pumps:
        labels = {"job": "gohome", "instance": "gohome", "heat_pump_id": pump["heatPumpId"], "serial_number": pump["serialNumber"], "model": pump.get("modelName", ""), "interval": "Hour"}
        if pump.get("name"):
            labels["name"] = pump["name"]
        day = args.start
        while day < args.end:
            end = min(day + dt.timedelta(days=7), args.end)
            for method, family in [("GetLogViews", "log"), ("GetEnergyLogs", "energy")]:
                archive = args.directory / (pump["heatPumpId"] + "-" + family + "-" + day.isoformat() + "-" + end.isoformat() + ".json")
                if archive.exists():
                    response = json.loads(archive.read_text())
                else:
                    response = rpc(args.host, service, method, {"heatPumpId": pump["heatPumpId"], "query": {"startTime": day.isoformat()+"T00:00:00Z", "endTime": end.isoformat()+"T00:00:00Z", "interval": "Hour"}})
                    archive.write_text(json.dumps(response))
                for raw in response.get("json", []):
                    row = json.loads(raw)
                    stamp = timestamp(row["timeBucket"])
                    for name, value in row.items():
                        if name in ("timeBucket", "interval") or value is None:
                            continue
                        if any(name in (prefix+"Min", prefix+"Max", prefix+"Average") and row.get(prefix+"Min") == -1 for prefix in ("tRoom", "tRoomTarget")):
                            continue
                        suffix = ""
                        if name in ("lastCoolingTimeMin", "lastCoolingTimeMax"):
                            value = timestamp(value)/1000
                            suffix = "_timestamp_seconds"
                        if type(value) not in (int, float, bool):
                            continue
                        metric = dict(labels, __name__="gohome_weheat_history_"+family+"_"+snake(name)+suffix)
                        yield metric, stamp, float(value)
            day = end


if __name__ == "__main__":
    recover("weheat", fetch_history)
