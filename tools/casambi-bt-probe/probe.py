"""Read-only Casambi authentication, state and reconnect smoke test."""

import argparse
import asyncio
import logging
import os
import time
from importlib.metadata import version
from pathlib import Path

import httpx
from CasambiBt import Casambi, discover


def arguments():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--address", required=True, help="Previously identified network MAC address")
    parser.add_argument("--network-name", required=True)
    parser.add_argument("--password-file", type=Path)
    parser.add_argument("--cache-directory", type=Path, required=True)
    parser.add_argument("--offline", action="store_true", help="Reject HTTP access; requires a populated cache")
    parser.add_argument("--observe-seconds", type=int, default=120)
    parser.add_argument("--connections", type=int, default=2)
    result = parser.parse_args()
    if not result.offline and result.password_file is None:
        parser.error("--password-file is required for initial authentication")
    if result.observe_seconds < 1 or result.connections < 1:
        parser.error("observation duration and connection count must be positive")
    return result


def report(key, value):
    print(f"{key}={value}", flush=True)


async def reject_http(request):
    raise RuntimeError("HTTP access disabled for offline proof")


async def smoke_test(options):
    report("bleak_version", version("bleak"))
    report("http_disabled", options.offline)
    devices = await discover()
    report("discovered_network_count", len(devices))
    matching_devices = [device for device in devices if device.address.upper() == options.address.upper()]
    if len(matching_devices) != 1:
        raise RuntimeError("Target network was not discovered")

    password = options.password_file.read_text().rstrip("\r\n") if options.password_file else ""
    for connection_number in range(1, options.connections + 1):
        transport = httpx.MockTransport(reject_http) if options.offline else None
        async with httpx.AsyncClient(transport=transport) as http_client:
            client = Casambi(httpClient=http_client, cachePath=options.cache_directory)
            observed_units = {}

            def record_state(unit):
                if unit.state is not None:
                    observed_units[unit.deviceId] = (unit.online, unit.is_on, unit.state.dimmer)

            client.registerUnitChangedHandler(record_state)
            try:
                report("connection_attempt", connection_number)
                await asyncio.wait_for(
                    client.connect(matching_devices[0], password, forceOffline=options.offline),
                    timeout=50,
                )
                if client.networkName != options.network_name:
                    raise RuntimeError("Authenticated network name did not match")
                report("bluetooth_authenticated", client.connected)
                report("configured_unit_count", len(client.units))
                started = time.monotonic()
                remaining = options.observe_seconds
                while remaining > 0:
                    await asyncio.sleep(min(30, remaining))
                    if not client.connected:
                        raise RuntimeError("Bluetooth connection dropped during observation")
                    elapsed = time.monotonic() - started
                    report("authenticated_seconds", round(elapsed))
                    remaining = options.observe_seconds - elapsed
                report("live_state_unit_count", len(observed_units))
                report("online_unit_count", sum(state[0] for state in observed_units.values()))
                report("brightness_read_count", sum(state[2] is not None for state in observed_units.values()))
                if not observed_units:
                    raise RuntimeError("Authenticated but no live unit state arrived")
            finally:
                await asyncio.wait_for(client.disconnect(), timeout=10)
        if connection_number < options.connections:
            await asyncio.sleep(2)


if __name__ == "__main__":
    os.umask(0o077)
    logging.disable(logging.CRITICAL)  # Upstream debug logs can include cryptographic material.
    options = arguments()
    try:
        timeout = 20 + options.connections * (options.observe_seconds + 65)
        asyncio.run(asyncio.wait_for(smoke_test(options), timeout=timeout))
    except Exception as error:
        # Do not print upstream exception bodies: they may include server responses.
        report("error_type", type(error).__name__)
        raise SystemExit(1) from None
