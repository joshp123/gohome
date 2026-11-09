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


def import_samples(vm, payload):
    # Keep requests below the reverse proxy's body limit. Complete JSONL rows
    # make interrupted imports resumable through the existing timestamp check.
    batch = bytearray()
    for line in payload.splitlines(keepends=True):
        if batch and len(batch) + len(line) > 512 * 1024:
            http(vm + "/api/v1/import", bytes(batch))
            batch.clear()
        batch.extend(line)
    if batch:
        http(vm + "/api/v1/import", bytes(batch))


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
        import_samples(args.vm, payload)
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


def source_read(args, resource, **fields):
    response = rpc(args.host, "gohome.plugins.tado.v1.TadoService", "GetSourceData", dict(resource=resource, **fields))
    import base64
    data = json.loads(base64.b64decode(response["json"]))
    return data, response


def require_history_budget(response):
    remaining = re.search(r"(?:^|;)\s*r=(\d+)", response.get("rateLimit", ""))
    reset = re.search(r"(?:^|;)\s*t=(\d+)", response.get("rateLimit", ""))
    # Reserve the collector's three GETs every 15 seconds, plus 100 calls for
    # startup discovery, retries and other clients. Unknown reset: reserve a day.
    reserve = (int(reset[1]) if reset else 86400) / 15 * 3 + 100
    if not remaining or int(remaining[1]) <= reserve:
        raise ValueError("insufficient verified quota for history; source responses remain archived")


def report_samples(report, labels):
    def point(name, stamp, value, extra=None):
        if value is None or not stamp:
            return
        metric = dict(labels, __name__="gohome_tado_history_"+name, **(extra or {}))
        yield metric, timestamp(stamp), float(value)
    measured = report.get("measuredData", {})
    for key, metric, unit in [("insideTemperature", "inside_temperature_celsius", "celsius"), ("humidity", "humidity_percent", "percentage")]:
        series = measured.get(key, {})
        for observation in series.get("dataPoints", []):
            value = observation.get("value")
            if isinstance(value, dict):
                value = value.get(unit)
            if value is not None and key == "humidity" and series.get("percentageUnit") == "UNIT_INTERVAL":
                value *= 100
            yield from point(metric, observation.get("timestamp"), value)
    for observation in measured.get("measuringDeviceConnected", {}).get("dataIntervals", []):
        yield from point("measuring_device_connected", observation.get("from"), observation.get("value"))
    for observation in report.get("callForHeat", {}).get("dataIntervals", []):
        if observation.get("value") and observation.get("from") and observation.get("to"):
            duration = (timestamp(observation["to"])-timestamp(observation["from"]))/1000
            if duration > 0:
                yield from point("call_for_heat_interval_seconds", observation["from"], duration, {"level": observation["value"]})
    for observation in report.get("stripes", {}).get("dataIntervals", []):
        value = observation.get("value") or {}
        if value.get("stripeType") and observation.get("from") and observation.get("to"):
            duration = (timestamp(observation["to"])-timestamp(observation["from"]))/1000
            if duration > 0:
                yield from point("mode_interval_seconds", observation["from"], duration, {"mode": value["stripeType"]})
        setting = value.get("setting") or {}
        yield from point("setpoint_celsius", observation.get("from"), (setting.get("temperature") or {}).get("celsius"))
        if setting.get("power") in ("ON", "OFF"):
            yield from point("power_on_bool", observation.get("from"), setting["power"] == "ON")
    for observation in report.get("settings", {}).get("dataIntervals", []):
        setting = observation.get("value") or {}
        yield from point("setpoint_celsius", observation.get("from"), (setting.get("temperature") or {}).get("celsius"))
        if setting.get("power") in ("ON", "OFF"):
            yield from point("power_on_bool", observation.get("from"), setting["power"] == "ON")
    for observation in report.get("weather", {}).get("condition", {}).get("dataIntervals", []):
        value = observation.get("value") or {}
        yield from point("outside_temperature_celsius", observation.get("from"), (value.get("temperature") or {}).get("celsius"))
        if value.get("state") and observation.get("to") and observation.get("from"):
            duration = (timestamp(observation["to"])-timestamp(observation["from"]))/1000
            if duration > 0:
                yield from point("weather_interval_seconds", observation["from"], duration, {"state": value["state"]})
    for observation in report.get("weather", {}).get("sunny", {}).get("dataIntervals", []):
        yield from point("sunny", observation.get("from"), observation.get("value"))


def fetch_history(args):
    zones, response = source_read(args, "ZONES")
    day = args.start
    while day < args.end:
        for zone in zones:
            if zone.get("dateCreated") and day < dt.date.fromisoformat(zone["dateCreated"][:10]):
                continue
            archive = args.directory / (str(zone["id"])+"-"+day.isoformat()+".json")
            if archive.exists():
                report = json.loads(archive.read_text())
            else:
                require_history_budget(response)
                report, response = source_read(args, "DAY_REPORT", zoneId=zone["id"], date=day.isoformat())
                archive.write_text(json.dumps(report))
            labels = {"job":"gohome", "instance":"gohome", "zone_id":str(zone["id"]), "zone_name":zone["name"]}
            yield from report_samples(report, labels)
        day += dt.timedelta(days=1)


if __name__ == "__main__":
    recover("tado", fetch_history)
