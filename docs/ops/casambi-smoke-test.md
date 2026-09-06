# Casambi local Bluetooth investigation

Written by AI on 2026.09.06 by model gpt-6-astra.

Status: Linux Bluetooth authentication, live state for all eight lights, offline process restart and reconnect proven. A separate HomeKit bridge now runs on the home Linux server. Secure diagnostic HomeKit pairing, encrypted state reads and pairing persistence across service restart are proven. Apple Home/Siri and physical control remain to be verified.

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
| Authentication | A newly supplied network password authenticated successfully through the network API, then over Bluetooth on Linux using BlueZ 5.87 and Bleak 3.0.2. |
| Current light state | Ten configured units comprise eight lights and two Xpress remotes. Live Bluetooth notifications supplied brightness for all eight lights; nine units reported state and eight were online. Battery remotes need not remain online. |
| Persistence/reconnect | Initial connect and reconnect passed. A fresh Linux process with HTTP disabled authenticated from the private cache, remained authenticated for 120 seconds, disconnected, reconnected, and read all eight lights again. This is a short smoke test, not long-term reliability proof. |

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

The user subsequently supplied a dedicated Casambi network credential. It is a network password entry, not a macOS login. Authentication now works. The Nix-provisioned reference client (Python 3.14.7, Bleak 3.0.2) successfully performed the Linux Bluetooth state/reconnect proof.

## Linux proof and operating choice

The Linux smoke used a temporary BlueZ 5.87 daemon on a private D-Bus socket. The existing host Bluetooth service was inactive. The probe enabled its unused radio, authenticated, observed notifications, and then stopped the temporary processes and restored the original controller settings. The earlier legacy `gatttool` error did not reproduce with the modern BlueZ/Bleak client; it was not evidence that the Linux hardware could not connect.

The reusable diagnostic is in `tools/casambi-bt-probe`, with a pinned Nix/devenv runtime and upstream source revision. It accepts an explicit network address/name, credential-file path and private cache directory. `--offline` rejects HTTP access through the client transport, so the process restart proof does not rely on an available cloud session endpoint. It never issues a light-control command.

Choose the existing home Linux host for the next service slice. It has demonstrated the full authenticated read path without a GUI session. The Mac mini has only demonstrated the native pre-authentication path and requires GUI-session authorization. Neither host has completed a long-running production soak; Linux currently has the stronger evidence and simpler NixOS/systemd operating model.

## HomeKit and Siri target

Expose the eight lights through one HomeKit bridge on the house LAN. Begin with on/off and brightness; add colour or colour temperature only when the device capabilities and readback are verified. The two Xpress remotes are not light accessories.

The chosen implementation is a standalone Python service combining the tested Casambi client with [HAP-python](https://github.com/ikalchev/HAP-python). This supersedes the initial Go adapter proposal. It owns the local Bluetooth connection and exposes HomeKit directly on the house LAN; Siri does not depend on GoHome being available. GoHome's existing `home` plugin remains a dashboard.

GoHome may become a client through a narrow protobuf API when there is an actual agent-control or telemetry consumer. No GoHome plugin or additional process boundary is needed for the HomeKit deliverable. Nix owns the standalone service configuration, agenix supplies the network password and HomeKit setup PIN, and private persistent service state holds downloaded keys and pairing material. There is no configuration UI.

Assign stable accessory IDs from stable device identity, not discovery order. Serve mDNS and a fixed HAP port on the house LAN. Report communication errors when state is unavailable; never present a stale value as a fresh read or acknowledge a failed control command.

Apple Home supplies Siri integration once the bridge is paired. Remote Siri access requires an Apple home hub such as HomePod or Apple TV; running the bridge on a Mac does not replace that role. [Apple guidance](https://support.apple.com/en-us/105027)

The supervised service boots with eight light accessories, authenticates from its cached keys, and preserves HomeKit identity and controller pairing across restart. A separate diagnostic IP controller completed mDNS discovery, secure pairing, and encrypted reads of all sixteen on/off and brightness characteristics. During Bluetooth startup it received communication errors instead of stale successful reads.

Restarting the Bluetooth daemon also recovered within the same bridge process, with all sixteen encrypted HomeKit reads passing again. The diagnostic controller removed its pairing after verification so Apple Home can become the owner.

Remaining proof: longer operation; current-iPhone Home pairing; one specifically agreed light-control/readback test. No physical light-control test has been performed.
