#!/usr/bin/env python3
"""Paced, resumable MIN history capture, import and verification.

Uses the hub's existing token over SSH; never stores or prints it. Requires the
Go import-file converter. Raw responses, backups and health evidence stay in a
private directory outside the repository. No device settings are written.
"""
import argparse
import datetime as dt
import fcntl
import json
import math
import os
from pathlib import Path
import re
import shlex
import subprocess
import time
import urllib.parse
import urllib.request
from zoneinfo import ZoneInfo


def http(url, params=None, body=None):
    if params:
        url += '?' + urllib.parse.urlencode(params)
    request = urllib.request.Request(url, data=body)
    with urllib.request.urlopen(request, timeout=60) as response:
        return response.read()


def saved_json(path, value):
    temporary = path.with_suffix('.tmp')
    temporary.write_text(json.dumps(value, indent=2) + '\n')
    temporary.replace(path)


def sample_key(labels, timestamp):
    return tuple(sorted((k, v) for k, v in labels.items() if v)), int(timestamp)


def import_samples(text):
    samples = {}
    for line in text.splitlines():
        series, value, timestamp = line.rsplit(' ', 2)
        name, labels = series.split('{', 1)
        labels = {k: json.loads(v) for k, v in re.findall(r'(\w+)=("(?:\\.|[^"\\])*")', labels)}
        labels['__name__'] = name
        key = sample_key(labels, timestamp)
        value = float(value)
        if key in samples and samples[key] != value:
            raise RuntimeError('Conflicting source readings at the same timestamp')
        samples[key] = value
    return samples


def exported_samples(payload):
    samples = {}
    for line in payload.splitlines():
        row = json.loads(line)
        for timestamp, value in zip(row['timestamps'], row['values']):
            samples[sample_key(row['metric'], timestamp)] = value
    return samples


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--start', required=True)
    parser.add_argument('--end', required=True)
    parser.add_argument('--directory', required=True, type=Path)
    parser.add_argument('--converter', required=True)
    parser.add_argument('--hub', required=True)
    parser.add_argument('--token-file', required=True)
    parser.add_argument('--vm', required=True)
    parser.add_argument('--serial', required=True)
    parser.add_argument('--plant-id', required=True)
    parser.add_argument('--plant-name', required=True)
    parser.add_argument('--timezone', default='Europe/Amsterdam')
    parser.add_argument('--interval-seconds', type=int, default=360)
    args = parser.parse_args()
    if args.interval_seconds < 360:
        parser.error('History requests must be at least six minutes apart')
    os.umask(0o077)
    args.directory.mkdir(parents=True, exist_ok=True, mode=0o700)
    lock = (args.directory / 'lock').open('w')
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    state_path = args.directory / 'state.json'
    state = json.loads(state_path.read_text()) if state_path.exists() else {'days': {}, 'last_request': 0}
    state['status'] = 'running'
    state.pop('error', None)
    state['pid'] = os.getpid()
    saved_json(state_path, state)
    local_zone = ZoneInfo(args.timezone)
    selector = '{__name__=~"gohome_growatt_inverter_.*",plant_id=' + json.dumps(args.plant_id) + ',serial=' + json.dumps(args.serial) + '}'

    def log(event, **fields):
        item = {'time': dt.datetime.now(dt.timezone.utc).isoformat(), 'event': event, **fields}
        with (args.directory / 'events.jsonl').open('a') as output:
            output.write(json.dumps(item) + '\n')
        print(json.dumps(item), flush=True)

    def health():
        query = '{__name__=~"gohome_growatt_(scrape_success|last_success_timestamp_seconds|last_update_timestamp_seconds|current_power_watts)",source="growatt"}'
        payload = json.loads(http(args.vm + '/api/v1/query', {'query': query}))
        if payload.get('status') != 'success':
            raise RuntimeError('VictoriaMetrics health query failed')
        readings = {r['metric']['__name__']: float(r['value'][1]) for r in payload['data']['result']}
        now = time.time()
        success = readings.get('gohome_growatt_scrape_success')
        age = now - readings.get('gohome_growatt_last_success_timestamp_seconds', 0)
        reading_age = now - readings.get('gohome_growatt_last_update_timestamp_seconds', 0)
        power = readings.get('gohome_growatt_current_power_watts')
        local = dt.datetime.now(local_zone)
        # Solar inverters stop reporting overnight. Still require successful cloud
        # reads; demand fresh electrical readings during the winter daylight core.
        daylight = 10 <= local.hour < 15
        log('health', scrape_success=success, last_success_age_seconds=round(age, 1),
            inverter_age_seconds=round(reading_age, 1), current_power_watts=power,
            require_daylight_power=daylight)
        if success != 1 or age > 720:
            raise RuntimeError('Live Growatt polling is unhealthy; stopping history requests')
        if daylight and (power is None or reading_age > 900):
            raise RuntimeError('Live daytime inverter readings are stale; stopping history requests')
        return age

    def export_day(day):
        start = dt.datetime.combine(day, dt.time.min, local_zone)
        end = start + dt.timedelta(days=1)
        return http(args.vm + '/api/v1/export', {
            'match[]': selector, 'start': start.isoformat(), 'end': (end - dt.timedelta(milliseconds=1)).isoformat()})

    try:
        health()
        backup = args.directory / 'before-all.jsonl'
        if not backup.exists():
            backup.write_bytes(http(args.vm + '/api/v1/export', {'match[]': selector}))
        day, end = dt.date.fromisoformat(args.start), dt.date.fromisoformat(args.end)
        while day <= end:
            date = day.isoformat()
            if date in state['days']:
                day += dt.timedelta(days=1)
                continue
            raw_path = args.directory / (date + '.json')
            if not raw_path.exists():
                while True:
                    age = health()
                    # Avoid the live poll's burst. Its last-success value advances
                    # on each five-minute fetch, not on the metrics scrape.
                    if time.time() - state['last_request'] >= args.interval_seconds and 90 <= age <= 220:
                        break
                    time.sleep(30)
                url = 'https://openapi.growatt.com/v4/new-api/queryHistoricalData?' + urllib.parse.urlencode({
                    'deviceSn': args.serial, 'deviceType': 'min', 'date': date})
                command = 'curl -fsS --max-time 45 -X POST -H "token: $(cat ' + shlex.quote(args.token_file) + ')" ' + shlex.quote(url)
                state['last_request'] = time.time()
                saved_json(state_path, state)
                log('request', date=date)
                result = subprocess.run(['ssh', '-o', 'BatchMode=yes', args.hub, command], capture_output=True, timeout=60)
                if result.returncode:
                    raise RuntimeError('History SSH/HTTP request failed; no automatic retry')
                raw_path.write_bytes(result.stdout)
            payload = json.loads(raw_path.read_bytes())
            if payload.get('code') == 20 and payload.get('message') == 'DATE_WITHIN_3_MONTHS':
                state['days'][date] = {'status': 'refused', 'reason': payload['message']}
                log('refused', date=date, reason=payload['message'])
            else:
                if payload.get('code') != 0:
                    raise RuntimeError('Growatt refused history: ' + str(payload.get('code')) + ' ' + str(payload.get('message')))
                if not isinstance(payload.get('data', {}).get('datas'), list):
                    raise RuntimeError('Missing history rows')
                # Reject data outside the requested local calendar day before any import.
                for row in payload['data']['datas']:
                    if not str(row.get('time', '')).startswith(date + ' '):
                        raise RuntimeError('History response contains a different date')
                result = subprocess.run([args.converter, '-input', str(raw_path), '-plant-id', args.plant_id,
                    '-plant-name', args.plant_name, '-serial', args.serial, '-timezone', args.timezone],
                    capture_output=True, check=True, timeout=60)
                text = result.stdout.decode().replace('{plant_id=', '{job="gohome",instance="gohome",source="growatt",plant_id=')
                expected = import_samples(text)
                (args.directory / (date + '.prom')).write_text(text)
                before_path = args.directory / (date + '-before.jsonl')
                if not before_path.exists():
                    before_path.write_bytes(export_day(day))
                before = exported_samples(before_path.read_bytes())
                for key in before.keys() & expected.keys():
                    if not math.isclose(before[key], expected[key], rel_tol=1e-12, abs_tol=1e-9):
                        raise RuntimeError('Conflicting pre-existing sample; not importing')
                if expected:
                    http(args.vm + '/api/v1/import/prometheus', body=text.encode())
                for attempt in range(12):
                    stored = export_day(day)
                    actual = exported_samples(stored)
                    if expected.keys() <= actual.keys():
                        break
                    time.sleep(5)
                if not expected.keys() <= actual.keys():
                    raise RuntimeError('Imported samples missing from verification export')
                for key, value in expected.items():
                    if not math.isclose(value, actual[key], rel_tol=1e-12, abs_tol=1e-9):
                        raise RuntimeError('Imported sample differs from source')
                if not before.keys() <= actual.keys() or any(actual[k] != v for k, v in before.items()):
                    raise RuntimeError('Pre-existing samples changed')
                (args.directory / (date + '-after.jsonl')).write_bytes(stored)
                summary = {'status': 'verified' if expected else 'empty', 'rows': len(payload['data']['datas']),
                    'samples': len(expected), 'series': len({k[0] for k in expected}),
                    'max_rounding_error': max((abs(value - actual[k]) for k, value in expected.items()), default=0)}
                state['days'][date] = summary
                log('verified', date=date, **summary)
            saved_json(state_path, state)
            health()
            day += dt.timedelta(days=1)
        # Observe the next live poll after the last history call before completion.
        while time.time() - state['last_request'] < 360:
            time.sleep(30)
            health()
        state['status'] = 'complete'
        log('complete', days=len(state['days']), samples=sum(d.get('samples', 0) for d in state['days'].values()))
    except Exception as error:
        state['status'] = 'stopped'
        state['error'] = str(error)
        log('stopped', error=str(error))
        raise
    finally:
        saved_json(state_path, state)


if __name__ == '__main__':
    main()
