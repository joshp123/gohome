# Casambi local Bluetooth investigation

Written by AI on 2026.09.06 by model gpt-6-astra.

Status: macOS discovery, GATT connection and authentication-challenge read proven on existing hardware; authenticated state and persistent operation unproven.

## Observed proof

| Stage | Result |
| --- | --- |
| Existing macOS radio | CoreBluetooth over SSH returns `unauthorized` (state 3), including when invoking the bundled executable directly. A dedicated app launched in the GUI session was authorized through Screen Sharing and scanned successfully. |
| Existing Linux radio | M8S host, Intel USB Bluetooth controller, `btusb` driver, `hci0`; neither hardware nor software blocked. Bluetooth daemon inactive and radio initially off. |
| Live Casambi discovery | BlueZ 5.84 captured manufacturer 963 and network service `0xfe4d`. The macOS app subsequently saw five distinct Casambi advertisers, including one network endpoint. |
| Network identity | Casambi network lookup returned HTTP 200; its name matched the user-supplied network name. This lookup is not authentication. |
| Signal | Approximately -89 dBm at the network endpoint on Linux; about -79 to -86 dBm on the Mac mini. Reach proven; sustained reliability remains untested. |
| GATT service discovery | Linux `gatttool --primary` returned `Request attribute has encountered an unlikely error`. The native macOS probe connected successfully and discovered one service. |
| Authentication characteristic | Linux read-by-UUID returned the same GATT error. macOS found the expected authentication characteristic and read a 25-byte challenge: type 1, protocol byte 43 (`0x2b`). It then disconnected cleanly. This is a pre-authentication read, not an authenticated session. |
| Authentication | Not attempted: network password unavailable. |
| Current light state | Not read. Advertisement presence is not light state. |
| Persistence/reconnect | Not tested. |

The Linux probe temporarily enabled LE and power, then restored both to their original disabled settings. The Mac mini already had Bluetooth on; the change was authorization for a dedicated probe app, not a radio power change. No pairing, commissioning, reset, or light-control command was issued. Device addresses, raw advertisements and credentials are deliberately excluded from this document.

## Protocol candidate

Inspected [casambi-bt](https://github.com/lkempf/casambi-bt/tree/ec23769ab3459abf8ba5f332267900964319d03e), package version 0.3.2. It is a reverse-engineered Python reference, not a proven GoHome dependency.

- Discovery requires both manufacturer 963 and service `0000fe4d-0000-1000-8000-00805f9b34fb`.
- Initial setup resolves the advertised address through Casambi's network API, authenticates with the network password, and downloads configuration/key material. Subsequent offline operation uses a cache; an empty cache cannot bootstrap offline.
- Bluetooth authentication uses `c9ffde48-ca5a-0001-ab83-8f519b482f77` for key exchange. The inspected implementation supports protocol versions 10–11 and is tested by its author with Evolution networks. The actual network type remains unknown.
- The upstream `demo.py` turns all lights on and off. Do not use it for discovery or read-only proof.
- Upstream reports include [authentication failure loops](https://github.com/lkempf/casambi-bt/issues/71) and [failures after extended operation](https://github.com/lkempf/casambi-bt/issues/69). These are reports against earlier integration versions, not proof that the inspected revision has the same failure.

## Reusable native probe

Build on macOS with Apple Command Line Tools already installed:

```sh
bash tools/casambi-macos-probe/build.sh "$HOME/Applications/GoHome Casambi Probe.app"
```

Launch the app from Finder in the GUI session and allow its Bluetooth prompt. It scans for ten seconds, connects only if exactly one Casambi network endpoint is present, discovers the authentication characteristic, reads the challenge, and disconnects. It sends no control or pairing commands. Results are written to `/tmp/gohome-casambi-app-result.txt` with private permissions; no device addresses or challenge contents are logged. The initial authorization window is bounded at two minutes, followed by a forty-second probe window once the radio is available.

The app identity is required for macOS authorization; adding an app bundle does not make a plain SSH invocation work. Rebuilding an ad-hoc-signed app can prompt for Bluetooth access again. A stable signed identity and verified GUI-session startup are prerequisites for a persistent macOS deployment.

## Credential search

No existing credential was recovered from the configured secrets, Apple Passwords metadata, full OpenTrawl house Notes and versions, or the former owner's handover messages. Handover media files were not archived locally, so that is a coverage gap. No passwords were guessed and no resets or sharing changes were made.

A Nix-provisioned Python runtime successfully imported the reference client dependencies (Python 3.14.7, Bleak 3.0.2). Bluetooth execution of that runtime remains unverified; the native Swift probe is the successful connection evidence.

## Next proof

1. Confirm firmware family and provide the password for the confirmed password-protected network through an agenix-managed secret, never a command argument, log, or chat message. Select the target network explicitly if multiple endpoints appear.
2. Provision a pinned reference client with Nix/devenv. Use a dedicated read-only probe, not the upstream demo. Keep downloaded keys/cache private; the library's cached material is sensitive too.
3. Authenticate, enumerate units, and receive current state from Bluetooth notifications. A successful cloud inventory fetch alone is insufficient.
4. Observe repeated state reports without issuing control commands. Disconnect only the probe's connection, reconnect, and prove fresh state again. Test process restart and cached bootstrap with cloud access disabled for the probe.
5. Establish sustained operation and recovery before selecting a transport implementation. Weak signal may require repositioning the existing host; no hardware purchase is justified yet.

## GoHome direction, conditional on proof

Prefer the existing home Linux host for a system service if a modern BlueZ client can prove the same connection path. The Mac mini already proves native GATT access, but its GUI-session authorization and startup requirements must be solved before treating it as a service host. The hosted GoHome server cannot be assumed to be in Bluetooth range. Use a narrow protobuf/gRPC boundary if a separate process is required; do not introduce a user interface or runtime configuration editor.

Once the operating path is proven, choose between a small Go protocol implementation and an isolated reference-client process based on observed compatibility and maintenance cost. Keep Nix as configuration source, agenix as credential source, plugin-owned protobuf/metrics/dashboard/agent context, and explicit freshness/connection status. No production plugin or persistent service is implemented yet.
