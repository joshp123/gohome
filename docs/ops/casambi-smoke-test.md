# Casambi local Bluetooth investigation

Written by AI on 2026.09.06 by model gpt-6-astra.

Status: discovery proven on existing hardware; authenticated state and persistent operation unproven.

## Observed proof

| Stage | Result |
| --- | --- |
| Existing macOS radio | Radio present; CoreBluetooth probe over SSH returned `unauthorized` (state 3). Zero advertisements is not evidence of absent lights. |
| Existing Linux radio | M8S host, Intel USB Bluetooth controller, `btusb` driver, `hci0`; neither hardware nor software blocked. Bluetooth daemon inactive and radio initially off. |
| Live Casambi discovery | BlueZ 5.84 management discovery and HCI monitor captured manufacturer 963 and network service `0xfe4d`. One network endpoint and another Casambi advertiser identified. |
| Signal | Approximately -89 dBm at the network endpoint and -91 dBm at the other advertiser. Reach proven; margin is weak. |
| GATT service discovery | A bounded `gatttool --primary` query returned `Request attribute has encountered an unlikely error`. This does not prove usable GATT services or authentication. |
| Authentication characteristic | A bounded read by the known authentication UUID returned the same GATT error. No challenge or authenticated session obtained. |
| Authentication | Not attempted: network password unavailable. |
| Current light state | Not read. Advertisement presence is not light state. |
| Persistence/reconnect | Not tested. |

The Linux probe temporarily enabled LE and power, then restored both to their original disabled settings. No pairing, commissioning, reset, or light-control command was issued. Device addresses, raw advertisements and credentials are deliberately excluded from this document.

## Protocol candidate

Inspected [casambi-bt](https://github.com/lkempf/casambi-bt/tree/ec23769ab3459abf8ba5f332267900964319d03e), package version 0.3.2. It is a reverse-engineered Python reference, not a proven GoHome dependency.

- Discovery requires both manufacturer 963 and service `0000fe4d-0000-1000-8000-00805f9b34fb`.
- Initial setup resolves the advertised address through Casambi's network API, authenticates with the network password, and downloads configuration/key material. Subsequent offline operation uses a cache; an empty cache cannot bootstrap offline.
- Bluetooth authentication uses `c9ffde48-ca5a-0001-ab83-8f519b482f77` for key exchange. The inspected implementation supports protocol versions 10–11 and is tested by its author with Evolution networks. The actual network type remains unknown.
- The upstream `demo.py` turns all lights on and off. Do not use it for discovery or read-only proof.
- Upstream reports include [authentication failure loops](https://github.com/lkempf/casambi-bt/issues/71) and [failures after extended operation](https://github.com/lkempf/casambi-bt/issues/69). These are reports against earlier integration versions, not proof that the inspected revision has the same failure.

## Next proof

1. Confirm network type/sharing and provide the network password through an agenix-managed secret, never a command argument, log, or chat message. Select the target network explicitly if multiple endpoints appear.
2. Provision a pinned reference client with Nix/devenv. Use a dedicated read-only probe, not the upstream demo. Keep downloaded keys/cache private; the library's cached material is sensitive too.
3. Authenticate, enumerate units, and receive current state from Bluetooth notifications. A successful cloud inventory fetch alone is insufficient.
4. Observe repeated state reports without issuing control commands. Disconnect only the probe's connection, reconnect, and prove fresh state again. Test process restart and cached bootstrap with cloud access disabled for the probe.
5. Establish sustained operation and recovery before selecting a transport implementation. Weak signal may require repositioning the existing host; no hardware purchase is justified yet.

## GoHome direction, conditional on proof

Run the radio-owning process on the existing home Linux host. The hosted GoHome server cannot be assumed to be in Bluetooth range. Use a narrow protobuf/gRPC boundary if a separate process is required; do not introduce a user interface or runtime configuration editor.

Once the operating path is proven, choose between a small Go protocol implementation and an isolated reference-client process based on observed compatibility and maintenance cost. Keep Nix as configuration source, agenix as credential source, plugin-owned protobuf/metrics/dashboard/agent context, and explicit freshness/connection status. No production plugin or persistent service is implemented yet.
