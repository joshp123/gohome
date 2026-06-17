#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <host>" >&2
  exit 1
fi

host="$1"

if [[ "${host}" == "gohome" ]]; then
  host="root@100.108.102.95"
fi

ssh "${host}" <<'SSH'
  set -euo pipefail

  export NIX_CONFIG='experimental-features = nix-command flakes'

  if [[ -d /root/gohome ]]; then
    repo="/root/gohome"
  else
    repo="/etc/nixos"
  fi

  if [[ "${repo}" == "/root/gohome" && ! -d "${repo}/.git" ]]; then
    if [[ ! -d /root/gohome-src/.git ]]; then
      git clone https://github.com/joshp123/gohome.git /root/gohome-src
    fi
    repo="/root/gohome-src"
  fi

  if [[ -d "${repo}/.git" ]]; then
    git -C "${repo}" fetch origin main
    if git -C "${repo}" merge-base --is-ancestor HEAD origin/main; then
      git -C "${repo}" merge --ff-only origin/main
    else
      deploy_repo="/root/gohome-deploy"
      if [[ ! -d "${deploy_repo}/.git" ]]; then
        git clone https://github.com/joshp123/gohome.git "${deploy_repo}"
      fi
      git -C "${deploy_repo}" fetch origin main
      if ! git -C "${deploy_repo}" diff --quiet || ! git -C "${deploy_repo}" diff --cached --quiet; then
        echo "deploy checkout ${deploy_repo} has local changes; refusing to overwrite" >&2
        exit 1
      fi
      git -C "${deploy_repo}" checkout --detach origin/main
      repo="${deploy_repo}"
    fi
  fi

  if [[ -d /root/nix-secrets ]]; then
    sudo nixos-rebuild switch --flake "${repo}#gohome" --override-input secrets /root/nix-secrets \
      || sudo nixos-rebuild switch --rollback
  else
    sudo nixos-rebuild switch --flake "${repo}#gohome" || sudo nixos-rebuild switch --rollback
  fi

  sleep 2
  curl -f http://localhost:8080/health || sudo nixos-rebuild switch --rollback
SSH
