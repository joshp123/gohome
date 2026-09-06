# Casambi Bluetooth smoke test

Written by AI on 2026.09.06 by model gpt-6-astra.

This diagnostic authenticates, observes state and reconnects. It sends no light-control, pairing, network-edit or commissioning commands. Run it on hardware already shown to receive the intended Casambi network.

Use the Nix/devenv environment in this directory. On Linux, BlueZ must be running and the adapter powered; the caller must have D-Bus access. A private diagnostic D-Bus session can be selected with `DBUS_SYSTEM_BUS_ADDRESS`. This tool does not change host services or adapter settings.

```sh
cd tools/casambi-bt-probe
devenv shell -- python probe.py \
  --address 'AA:BB:CC:DD:EE:FF' \
  --network-name 'Example' \
  --password-file /run/agenix/casambi-password \
  --cache-directory /var/lib/gohome-casambi/cache
```

Then start a new process with the same cache and `--offline`, omitting the password file. Offline mode rejects all requests made through the client's HTTP transport; it does not alter the host's network access. The default observes each of two authenticated connections for two minutes.

Success requires live Bluetooth state, not just cloud inventory. The summary reports configured units separately from observed units, online units and units reporting brightness. Battery remotes need not report continuously. No device names, addresses, passwords, keys or challenge bytes are printed.

The cache contains credentials and downloaded key material. Keep its parent directory private and persistent. The library is pinned to the revision tested in this investigation; it remains an unofficial reference client, not a production GoHome plugin. Production reconnect supervision and HomeKit pairing are separate deliverables.
