#!/usr/bin/env python3
"""Remove copies of stamped readings that VictoriaMetrics stored at scrape time instead of at their own time.

Daikin's electricity buckets and Roborock's run records carry their own timestamp: a bucket's start, a run's end.
While the scrape jobs ignored those timestamps, every 15-second scrape stored another copy of the latest bucket or
run at the scrape time. Those copies are wrong; the samples at the readings' own times are right. VictoriaMetrics
cannot delete a time range, so this exports the series, keeps the right samples, deletes the series and imports the
kept samples back.

Run it after the scrape jobs honour timestamps (nix/victoriametrics.nix), so no new copies arrive. Without --apply
it only exports, backs up and reports what it would remove. Agent-written (claude opus 5.5, 2026.11.08).
"""
import argparse
import datetime
import gzip
import hashlib
import json
import pathlib
import urllib.parse
import urllib.request

# Each source: the series to repair, and whether a sample's stored time is its reading's own time.
HOUR_MS = 3_600_000


def daikin_rule(rows):
    # A bucket starts on a whole local hour; where the zone's offset is whole hours, that is a whole UTC hour.
    return lambda row, ts, value: ts % HOUR_MS == 0


def roborock_rule(rows):
    # A run record is stamped with the run's end, which the end-time family also holds as its value.
    ends = {ts for row in rows if row["metric"]["__name__"] == "gohome_roborock_cleaning_run_end_timestamp_seconds"
            for ts, value in zip(row["timestamps"], row["values"]) if ts == int(value) * 1000}
    return lambda row, ts, value: ts in ends


SOURCES = {
    "daikin": ("gohome_daikin_electricity_consumption_kwh", daikin_rule),
    "roborock": ('{__name__=~"gohome_roborock_cleaning_run_.*"}', roborock_rule),
}


def http(url, data=None, method=None):
    request = urllib.request.Request(url, data=data, method=method)
    with urllib.request.urlopen(request, timeout=300) as response:
        return response.read()


def export(vm, selector):
    body = http(vm + "/api/v1/export?" + urllib.parse.urlencode({"match[]": selector}))
    return body, [json.loads(line) for line in body.splitlines() if line.strip()]


def split(rows, right, since_ms):
    """(kept rows, per-series report). Only samples stored at or after since_ms can be dropped."""
    kept, report = [], []
    for row in rows:
        good, dropped, seen = [], [], {}
        for ts, value in zip(row["timestamps"], row["values"]):
            if right(row, ts, value):
                if ts in seen and seen[ts] != value:
                    raise SystemExit(f"two values at one time in {row['metric']}: {ts}; nothing changed")
                if ts not in seen:
                    seen[ts] = value
                    good.append((ts, value))
            elif ts < since_ms:
                raise SystemExit(f"a wrong-looking sample from before --since in {row['metric']} at {ts}; nothing changed")
            else:
                dropped.append(ts)
        kept.append({"metric": row["metric"], "timestamps": [t for t, _ in good], "values": [v for _, v in good]})
        report.append({"metric": row["metric"], "stored": len(row["timestamps"]), "kept": len(good), "dropped": len(dropped),
                       "dropped_from": min(dropped, default=None), "dropped_to": max(dropped, default=None)})
    return kept, report


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("source", choices=sorted(SOURCES))
    parser.add_argument("--vm", required=True, help="VictoriaMetrics base URL, for example http://<hub>/vm")
    parser.add_argument("--since", required=True, help="when the copies began (the deploy), ISO 8601 with offset")
    parser.add_argument("--backup-dir", required=True, type=pathlib.Path, help="a private directory, not in git")
    parser.add_argument("--apply", action="store_true", help="delete and re-import; without it, report only")
    args = parser.parse_args()
    vm = args.vm.rstrip("/")
    selector, rule = SOURCES[args.source]
    since_ms = int(datetime.datetime.fromisoformat(args.since).timestamp() * 1000)

    body, rows = export(vm, selector)
    if not rows:
        raise SystemExit("no series match; nothing to do")
    kept, report = split(rows, rule(rows), since_ms)
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    backup = args.backup_dir / f"{args.source}-{stamp}"
    backup.mkdir(parents=True, mode=0o700)
    (backup / "full.jsonl.gz").write_bytes(gzip.compress(body))
    (backup / "full.sha256").write_text(hashlib.sha256(body).hexdigest() + "\n")
    kept_body = "".join(json.dumps(row) + "\n" for row in kept if row["timestamps"]).encode()
    (backup / "kept.jsonl").write_bytes(kept_body)
    (backup / "report.json").write_text(json.dumps(report, indent=1))
    stored, keep, drop = (sum(r[k] for r in report) for k in ("stored", "kept", "dropped"))
    print(f"{args.source}: {len(rows)} series, {stored} samples stored, {keep} kept, {drop} to remove; backup {backup}")
    if not args.apply:
        return

    # Export again just before deleting: what changed since the backup was taken must be in it.
    body, rows = export(vm, selector)
    (backup / "full-at-delete.jsonl.gz").write_bytes(gzip.compress(body))
    kept, report = split(rows, rule(rows), since_ms)
    newest_copy = max((r["dropped_to"] for r in report if r["dropped_to"]), default=0)
    if newest_copy > datetime.datetime.now(datetime.timezone.utc).timestamp() * 1000 - 120_000:
        raise SystemExit("copies are still arriving: the scrape job does not honour timestamps yet; nothing changed")
    kept_body = "".join(json.dumps(row) + "\n" for row in kept if row["timestamps"]).encode()
    (backup / "kept-at-delete.jsonl").write_bytes(kept_body)
    http(vm + "/api/v1/admin/tsdb/delete_series?" + urllib.parse.urlencode({"match[]": selector}), method="POST")
    http(vm + "/api/v1/import", data=kept_body, method="POST")
    # Range queries cache results older than a few minutes; drop the cache so panels read the repaired history.
    http(vm + "/internal/resetRollupResultCache")

    _, after = export(vm, selector)
    stored_now = {(json.dumps(r["metric"], sort_keys=True), t): v for r in after for t, v in zip(r["timestamps"], r["values"])}
    missing = [(row["metric"], t) for row in kept for t, v in zip(row["timestamps"], row["values"])
               if stored_now.get((json.dumps(row["metric"], sort_keys=True), t)) != v]
    _, report_after = split(after, rule(after), since_ms)
    still_wrong = sum(r["dropped"] for r in report_after)
    print(f"after: {sum(len(r['timestamps']) for r in after)} samples stored, {len(missing)} kept samples missing, "
          f"{still_wrong} stored at the wrong time")
    if missing or still_wrong:
        raise SystemExit(f"verification failed; restore from {backup}/full-at-delete.jsonl.gz")


if __name__ == "__main__":
    main()
