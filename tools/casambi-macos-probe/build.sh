#!/usr/bin/env bash
set -euo pipefail

source_directory="$(cd "$(dirname "$0")" && pwd)"
app_directory="${1:?Usage: build.sh '/absolute/path/GoHome Casambi Probe.app'}"
mkdir -p "$app_directory/Contents/MacOS"
cp "$source_directory/Info.plist" "$app_directory/Contents/Info.plist"
xcrun swiftc "$source_directory/main.swift" -o "$app_directory/Contents/MacOS/casambi-probe"
codesign --force --sign - "$app_directory"
