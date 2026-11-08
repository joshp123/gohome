#!/usr/bin/env python3
"""Import the energy export without replacing existing history or other series."""
import argparse
import datetime
import json
from pathlib import Path
import urllib.parse
import urllib.request

METRIC = "gohome_daikin_electricity_consumption_kwh"


def points(payload):
    result = {}
    for line in payload.splitlines():
        row = json.loads(line)
        if row["metric"].get("__name__") != METRIC:
            raise ValueError("input contains a different metric")
        if len(row["values"]) != len(row["timestamps"]):
            raise ValueError("value/timestamp count differs")
        labels = tuple(sorted(row["metric"].items()))
        for timestamp, value in zip(row["timestamps"], row["values"]):
            key = (labels, timestamp)
            if key in result and result[key] != value:
                raise ValueError("conflicting existing samples")
            result[key] = value
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True, type=Path)
    parser.add_argument("--vm-url", default="http://127.0.0.1:8428/vm")
    parser.add_argument("--backup-dir", required=True, type=Path)
    args = parser.parse_args()
    incoming = args.input.read_bytes()
    requested = points(incoming)
    if not requested:
        raise ValueError("empty import")
    backup = args.backup_dir / datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S.%fZ")
    backup.mkdir(parents=True, mode=0o700)
    (backup / "incoming.jsonl").write_bytes(incoming)
    query = urllib.parse.urlencode({"match[]": METRIC, "reduce_mem_usage": "1"})
    export_url = args.vm_url.rstrip("/") + "/api/v1/export?" + query
    with urllib.request.urlopen(export_url, timeout=60) as response:
        previous = response.read()
    (backup / "previous.jsonl").write_bytes(previous)
    existing = points(previous)
    missing = []
    for key, value in requested.items():
        if key in existing:
            if existing[key] != value:
                raise ValueError(f"existing value differs at {key[1]}; no import performed")
            continue
        labels, timestamp = key
        missing.append(json.dumps({"metric": dict(labels), "values": [value], "timestamps": [timestamp]}))
    if not missing:
        print(f"All {len(requested)} samples already stored; backup {backup}")
        return
    payload = ("\n".join(missing) + "\n").encode()
    request = urllib.request.Request(args.vm_url.rstrip("/") + "/api/v1/import", data=payload,
                                     headers={"Content-Type": "application/json"}, method="POST")
    with urllib.request.urlopen(request, timeout=60) as response:
        response.read()
    print(f"Imported {len(missing)} samples; backup {backup}")


if __name__ == "__main__":
    main()
