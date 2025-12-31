#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <host>" >&2
  exit 1
fi

host="$1"

ssh "${host}" <<'SSH'
  set -euo pipefail
  cd /etc/nixos
  git pull origin main
  sudo nixos-rebuild switch --flake .#gohome || sudo nixos-rebuild switch --rollback
  curl -f http://localhost:8080/health || sudo nixos-rebuild switch --rollback
SSH
