#!/usr/bin/env bash
# Installs a built gohome-applehome.app as ~/Applications/gohome-applehome.app and
# restarts the running node from it. The login item (a LaunchAgent with KeepAlive,
# see README.md) starts the new bundle once the old process exits.
#
#   ./install.sh .build/xcode/Build/Products/Release-maccatalyst/gohome-applehome.app
#
# The installed bundle it replaces becomes gohome-applehome.previous.app; to roll
# back, run this script with that bundle.
set -euo pipefail

source_app="${1:?usage: install.sh <path to gohome-applehome.app>}"
installed="$HOME/Applications/gohome-applehome.app"
previous="$HOME/Applications/gohome-applehome.previous.app"
staged="$HOME/Applications/gohome-applehome.staged.app"

codesign --verify --deep --strict "$source_app"

# ditto merges into an existing bundle, so always copy into a fresh path.
[ ! -e "$staged" ] || trash "$staged"
ditto "$source_app" "$staged"
codesign --verify --deep --strict "$staged"

if [ -e "$installed" ]; then
  [ ! -e "$previous" ] || trash "$previous"
  mv "$installed" "$previous"
fi
mv "$staged" "$installed"

# Ends the node started from this path, whichever bundle it now points at.
pkill -TERM -f "^$installed/Contents/MacOS/gohome-applehome( |$)" || true
echo "installed $installed; the login item restarts it within about 10 seconds"
