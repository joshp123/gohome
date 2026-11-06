# Meaco — Agent Context

Written by AI on 2026.11.06 by model gpt-6-astra.

## What This Is

Local Tuya 3.3 integration for the Meaco Arete Two 25L. No cloud dependency.
Bootstrap credentials come from agenix, never Grafana or runtime edits.

## Capabilities

Reads fresh device status and exposes typed gRPC controls for power, target
humidity and mode. Every request connects again, so a disconnected device is
retried automatically on the next scrape or API call. Requests are serialized.

## Limits

This device is reported to disconnect from Wi-Fi frequently. A failed read
exports no Meaco metrics, including no retained last-success metric. There is
no cached device state. Registry health is degraded until a read succeeds,
after any read failure, or when the last successful read is over 90 seconds old.
The client bounds each network exchange to five seconds.

Use `deviceIp` in Nix to override
the bootstrap's last-known IP after a reservation is approved. Do not scan or
change the router automatically. The host needs a working route to the device's LAN
and permission to reach the device's LAN address on TCP 6668.

## Methods

- `GetStatus`: performs a live read. Optional fields remain absent if omitted.
- `SetPower`: sets DPS 1, only if the preceding read reports it. This field was
  absent from recorded device data; power control remains unproven.
- `SetTargetHumidity`: DPS 2, 35–70 percent in steps of five.
- `SetMode`: DPS 4, manual, laundry, sleep or purify.

Controls first read the device and reject unsupported fields. An acknowledgement
is not a verified state change: read status afterwards. Do not blindly retry a
write after a timeout; its outcome is unknown. Never reset or re-pair this device
to repair connectivity: pairing may rotate the local key.

## State

A local recording reported these DPS values:

| DPS | Meaning |
| --- | --- | --- |
| 2 | Target humidity |
| 4 | Mode |
| 14 | Child lock |
| 16 | Indoor humidity |
| 17 | Countdown |
| 19 | Raw status/fault bitmap |
| 101 | On timer |

recorded read and same-target write result. These are historical mapping evidence,
not current readings. No compressor-running or tank-full value was
observed. Export raw DPS 19 rather than infer either from humidity or power.
Supported mode values, target range and optional DPS 1 match the product entry
in [tuya-local's Meaco definition](https://github.com/make-all/tuya-local/blob/main/custom_components/tuya_local/devices/meaco_aretetwo_dehumidifier.yaml).

Metrics use `gohome_meaco_` with `humidity_percent`, `target_humidity_percent`,
`power`, `child_lock`, `fault_code`, `mode{value}`, `countdown{value}`,
`on_timer{value}` and `last_success_timestamp_seconds`. Every exported value
comes from that scrape. Dashboard current-state panels use instant queries;
humidity history does not connect missing readings.

## Errors

Connection refused or timeout: device offline, address changed, route unavailable
or ACL restriction. Next read reconnects. Decryption failure: check the local
key; do not log bootstrap contents. Missing DPS: no values are invented.

## Verification and deployment gate

Do not deploy until a live status read
and one same-value target-humidity write succeed, with a confirming read.
