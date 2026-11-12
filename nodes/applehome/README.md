# Apple Home node

Written by AI on 2026.11.07 by model claude opus 5.5.

A small Mac app that reads every accessory in Apple Home through
Apple's HomeKit framework, read-only, and serves it as a gohome node: the gohome
registry, the `applehome` gRPC service (`proto/plugins/applehome.proto`) with
reflection, `/metrics/applehome` and `/health`. What it offers agents is in
`Sources/AppleHomeNode/Resources/AGENTS.md`. The gohome hub scrapes it through
`services.gohome.remotePlugins`, like a plugin another gohome runs.

## Why it looks like this

- **Mac Catalyst.** macOS has no HomeKit framework for ordinary Mac apps, up to and
  including the macOS 27 SDK; Apple ships it to Mac Catalyst apps only.
- **Apple Development signing.** Apple grants the HomeKit entitlement on the Mac
  only to development-signed builds and Mac App Store builds, not Developer ID. A
  development build runs only on Macs registered on the team, so the Mac must
  be a registered device and the provisioning profile must list it.
- **Two halves.** `Sources/AppleHomeNode` (Swift package) is the server: registry,
  gRPC, metrics. It has no HomeKit code and builds with `swift build`.
  `App/` is the Catalyst app: the HomeKit reader and the entry point.
- **Read-only by construction.** The reader calls `readValue` and
  `enableNotification` only. There is no `writeValue` anywhere in the code.

## Layout

| Path | What |
| --- | --- |
| `App/HomeKitReader.swift` | Polls every readable characteristic once a minute and takes change notifications |
| `App/AppDelegate.swift` | Windowless app entry; settings from launch arguments |
| `Sources/AppleHomeNode/` | Node server, metrics, health; `Generated/` is protoc output |
| `Sources/AppleHomeNode/Resources/` | AGENTS.md, reflection descriptor set |
| `Tests/` | Package tests: stale and failed reads never reported as current |
| `project.yml` | XcodeGen spec for the Catalyst app |
| `generate.sh` | Regenerates `Generated/` and the descriptor set from `proto/` |
| `install.sh` | Installs a build into `~/Applications` and restarts the node from it |

## Build

Needs Xcode 27 and Nix.

```
cd nodes/applehome
./generate.sh                                   # after changing a proto
GOHOME_APPLE_TEAM_ID=<team ID> \
nix run nixpkgs#xcodegen -- generate            # makes AppleHomeNode.xcodeproj
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer \
xcodebuild -project AppleHomeNode.xcodeproj -scheme GohomeAppleHome \
  -destination 'platform=macOS,variant=Mac Catalyst' -configuration Release \
  -derivedDataPath .build/xcode -allowProvisioningUpdates build
```

Test the server half on plain macOS:

```
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer swift test
```

An unsigned build (`CODE_SIGNING_ALLOWED=NO`) runs and serves the node, but HomeKit
gives it nothing: registry status stays DEGRADED, "Waiting for Home access or the
first read".

## Signing

Signing needs an Apple Development certificate and a Mac Catalyst development
provisioning profile, from the team whose ID `GOHOME_APPLE_TEAM_ID` gives, for the
App ID in `project.yml`, with the HomeKit capability,
that lists the Mac the node runs on. Xcode's automatic signing makes all of them
when Xcode on that Mac is signed in to the team's Apple Account (Settings, Apple
Accounts) and the build passes `-allowProvisioningUpdates`; it also renews the
profile, which lasts a year.

If the App ID does not exist yet, `xcodebuild` alone fails with "Entitlement
com.apple.developer.homekit not found". Open the project in Xcode once and select
the target's Signing & Capabilities tab: Xcode registers the App ID with HomeKit
and makes the profile. After that, `xcodebuild` works on its own.

The app signs with a team-based designated requirement (`OTHER_CODE_SIGN_FLAGS` in
`project.yml`), so the Home permission survives certificate renewal.

Delete the built `.app` before a build that follows a profile change. An
incremental build copies the new profile without re-signing, and the bundle then
fails `codesign --verify --strict`.

Check a build:

```
codesign -dv --verbose=4 gohome-applehome.app    # Authority=Apple Development, TeamIdentifier=<team ID>
codesign -d --entitlements - gohome-applehome.app # com.apple.developer.homekit
codesign -d -r- gohome-applehome.app              # the team-based requirement
codesign --verify --deep --strict gohome-applehome.app
security cms -D -i gohome-applehome.app/Contents/embedded.provisionprofile  # devices, HomeKit, expiry
```

## Run

In production the node runs as a user LaunchAgent, so it starts at login and
restarts if it exits. Its program is `open`, which starts the app through Launch
Services, hidden and in the background:

```
/usr/bin/open -W -g -j -a ~/Applications/gohome-applehome.app \
  --stdout <log> --stderr <log> \
  --args -listenHost <tailnet address of the Mac> -grpcPort 9000 -httpPort 8080
```

with `RunAtLoad` and `KeepAlive`. Launched this way the app is its own responsible
process, so the Home permission granted to it applies; started from a shell, the
shell is responsible and HomeKit gives the app no homes. `-j` keeps the empty
window that every Mac Catalyst app gets hidden, `-g` keeps it from taking focus,
and `-W` keeps `open` running until the app exits, so `KeepAlive` restarts it.

Install a build with `./install.sh <built .app>`: it verifies the signature,
copies the build to `~/Applications/gohome-applehome.app` (keeping the bundle it
replaces as `gohome-applehome.previous.app`) and ends the running node, which the
LaunchAgent then starts from the new bundle. Roll back by running it with the
previous bundle.

The first run asks, on the Mac's screen, to allow access to Apple Home. macOS
shows one permission prompt at a time, so an unanswered prompt from another app
holds this one back. If an earlier build with a different signature ran, macOS
first asks whether this build may open that build's data. Later runs need no
answer: the permission follows the team-based requirement.

Check it:

```
grpcurl -plaintext <host>:9000 gohome.registry.v1.Registry/ListPlugins
grpcurl -plaintext <host>:9000 gohome.plugins.applehome.v1.AppleHomeService/GetClimate
curl http://<host>:8080/metrics/applehome
```
