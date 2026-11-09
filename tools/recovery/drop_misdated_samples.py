#!/usr/bin/env python3
"""Remove copies of stamped readings that VictoriaMetrics stored at scrape time instead of at their own time.

Daikin's electricity buckets and Roborock's run records carry their own timestamp: a bucket's start, a run's end.
While the scrape jobs ignored those timestamps, every 15-second scrape stored another copy of the latest bucket or
run at the scrape time. Those copies are wrong; the samples at the readings' own times are right. VictoriaMetrics
cannot delete a time range, so this exports the series, keeps the right samples, deletes the series and imports the
kept samples back.

Run it after the scrape jobs honour timestamps (nix/victoriametrics.nix), so no new copies arrive. Without --apply
it only exports, backs up and reports what it would remove. Agent-written (claude opus 5.5, 2026.11.08).

The same export, back-up, delete and re-import also removes a heat pump's frozen counter readings
(source weheat-frozen): see frozen_run_rule. Agent-written (claude opus 5.5, 2026.11.09).
"""
import argparse
import datetime
import gzip
import hashlib
import json
import pathlib
import time
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


FROZEN_RUN_MAX_MS = 14 * 24 * HOUR_MS


def frozen_run_rule(since_ms, until_ms):
    """Drop a counter's readings from its jump at since_ms until until_ms, when it updates normally again.

    Weheat's API can return stale energy counters: the counters jump to lower values, may come back to about the
    right level once, then stay unchanged until they update normally again, while the heat pump's other
    readings went on changing and Weheat's recovered hourly history shows it working throughout. Every counter
    reading in that window is false or stale; the history covers every hour of it. A series without a jump at
    since_ms keeps everything, and a series that changed more than a few times in the window is refused, since that
    is a working counter.
    """
    if not until_ms or not 0 < until_ms - since_ms <= FROZEN_RUN_MAX_MS:
        raise SystemExit("--until is needed, after --since and within two weeks of it")

    def rule(rows):
        false = set()
        for row in rows:
            ts, vs = row["timestamps"], row["values"]
            i = next((k for k, t in enumerate(ts) if t >= since_ms), None)
            # The jump is the first reading at or after since_ms, within a minute of it.
            if i is None or i == 0 or ts[i] - since_ms > 60_000 or vs[i] == vs[i - 1]:
                continue
            values_in_window = {v for t, v in zip(ts, vs) if since_ms <= t < until_ms}
            if len(values_in_window) > 3:
                raise SystemExit(f"{row['metric']['__name__']}: {len(values_in_window)} different readings between --since and --until; "
                                 "a counter that changes is working; nothing changed")
            false.add(json.dumps(row["metric"], sort_keys=True))

        def keep(row, ts, value):
            return not (json.dumps(row["metric"], sort_keys=True) in false and since_ms <= ts < until_ms)
        return keep
    return rule


SOURCES = {
    "daikin": ("gohome_daikin_electricity_consumption_kwh", daikin_rule),
    "roborock": ('{__name__=~"gohome_roborock_cleaning_run_.*"}', roborock_rule),
    # Run with --since <the first false reading> --until <the first normal reading>, ISO 8601 with offset.
    "weheat-frozen": ('{__name__=~"gohome_weheat_energy_total_(ein|eout)_(heating|dhw|heating_defrost|dhw_defrost|standby)_kwh"}', None),
}


def http(url, data=None, method=None):
    request = urllib.request.Request(url, data=data, method=method)
    with urllib.request.urlopen(request, timeout=300) as response:
        return response.read()


def export(vm, selector):
    body = http(vm + "/api/v1/export?" + urllib.parse.urlencode({"match[]": selector}))
    return body, [json.loads(line) for line in body.splitlines() if line.strip()]


def matched_series(vm, selector):
    # VictoriaMetrics reads start=0 as its default (the last day), so ask from 2000 to reach series that have stopped.
    body = http(vm + "/api/v1/series?" + urllib.parse.urlencode({"match[]": selector, "start": 946684800}))
    return json.loads(body)["data"]


def exact_selector(vm, labels):
    """A selector that matches the series with exactly these labels and no other.

    A selector leaves unnamed labels free, so a series with one more label (the live copy of a counter, with its
    source label) would match too. Each label another matched series has and this one lacks is required empty, and
    the result must match this series alone.
    """
    quote = lambda v: '"' + v.replace("\\", "\\\\").replace('"', '\\"') + '"'
    names = dict(labels)
    for other in matched_series(vm, "{" + ",".join(f"{k}={quote(v)}" for k, v in sorted(names.items())) + "}"):
        for k in other:
            names.setdefault(k, "")
    selector = "{" + ",".join(f"{k}={quote(v)}" for k, v in sorted(names.items())) + "}"
    matches = matched_series(vm, selector)
    if [sorted(m.items()) for m in matches] != [sorted(labels.items())]:
        raise SystemExit(f"{selector} matches {len(matches)} series, not just the one to repair; nothing changed")
    return selector


def import_rows(vm, rows):
    """Import rows in requests the hub's nginx accepts (512 KiB, as plugins/weheat/recover_history.py does).

    VictoriaMetrics also refuses a line over 10 MiB, which one long series is, so each series goes in lines of at
    most 10 000 samples.
    """
    batch = bytearray()
    for row in rows:
        for k in range(0, len(row["timestamps"]), 10_000):
            line = (json.dumps({"metric": row["metric"], "timestamps": row["timestamps"][k:k + 10_000],
                                "values": row["values"][k:k + 10_000]}) + "\n").encode()
            if batch and len(batch) + len(line) > 512 * 1024:
                http(vm + "/api/v1/import", data=bytes(batch), method="POST")
                batch.clear()
            batch.extend(line)
    if batch:
        http(vm + "/api/v1/import", data=bytes(batch), method="POST")


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
    parser.add_argument("--until", help="for weheat-frozen: when the counters updated normally again, ISO 8601 with offset")
    parser.add_argument("--backup-dir", required=True, type=pathlib.Path, help="a private directory, not in git")
    parser.add_argument("--apply", action="store_true", help="delete and re-import; without it, report only")
    args = parser.parse_args()
    vm = args.vm.rstrip("/")
    selector, rule = SOURCES[args.source]
    since_ms = int(datetime.datetime.fromisoformat(args.since).timestamp() * 1000)
    if rule is None:
        until_ms = int(datetime.datetime.fromisoformat(args.until).timestamp() * 1000) if args.until else None
        rule = frozen_run_rule(since_ms, until_ms)

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
    # Only the series that lose samples are deleted and re-imported, each matched by its exact labels, so series that
    # are still being collected are never touched.
    affected = [(row, original) for row, original, r in zip(kept, rows, report) if r["dropped"]]
    newest = max((original["timestamps"][-1] for _, original in affected), default=0)
    if newest > datetime.datetime.now(datetime.timezone.utc).timestamp() * 1000 - HOUR_MS:
        raise SystemExit("a series to repair has a reading from the last hour: it is still being collected; nothing changed")
    kept_body = "".join(json.dumps(row) + "\n" for row, _ in affected if row["timestamps"]).encode()
    (backup / "kept-at-delete.jsonl").write_bytes(kept_body)
    selectors = [exact_selector(vm, row["metric"]) for row, _ in affected]
    for selector_one in selectors:
        http(vm + "/api/v1/admin/tsdb/delete_series?" + urllib.parse.urlencode({"match[]": selector_one}), method="POST")
    kept = [row for row, _ in affected]
    import_rows(vm, kept)
    # Range queries cache results older than a few minutes; drop the cache so panels read the repaired history.
    http(vm + "/internal/resetRollupResultCache")

    # Imported samples become searchable only after VictoriaMetrics flushes them (a few seconds), so check until they
    # all show or a minute has passed.
    deadline = time.monotonic() + 60
    while True:
        _, after = export(vm, selector)
        stored_now = {(json.dumps(r["metric"], sort_keys=True), t): v for r in after for t, v in zip(r["timestamps"], r["values"])}
        untouched = [original for original, r in zip(rows, report) if not r["dropped"]]
        missing = [(row["metric"], t) for row in kept + untouched for t, v in zip(row["timestamps"], row["values"])
                   if stored_now.get((json.dumps(row["metric"], sort_keys=True), t)) != v]
        _, report_after = split(after, rule(after), since_ms)
        still_wrong = sum(r["dropped"] for r in report_after)
        if not (missing or still_wrong) or time.monotonic() > deadline:
            break
        time.sleep(2)
    print(f"after: {sum(len(r['timestamps']) for r in after)} samples stored, {len(missing)} kept samples missing, "
          f"{still_wrong} stored at the wrong time")
    if missing or still_wrong:
        raise SystemExit(f"verification failed; restore from {backup}/full-at-delete.jsonl.gz")


if __name__ == "__main__":
    main()
