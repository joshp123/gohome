#!/usr/bin/env bash
set -euo pipefail

curl -fsSL https://raw.githubusercontent.com/elitak/nixos-infect/master/nixos-infect | bash -s -- -y
