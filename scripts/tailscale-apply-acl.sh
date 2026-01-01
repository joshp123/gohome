#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
ACL_FILE="${ROOT}/infra/tailscale/acl.json"

if [[ ! -f "${ACL_FILE}" ]]; then
  echo "Missing ACL file: ${ACL_FILE}" >&2
  exit 1
fi

if [[ -z "${TAILSCALE_API_KEY:-}" ]]; then
  if [[ -n "${NIX_SECRETS_DIR:-}" && -f "${NIX_SECRETS_DIR}/tailscale-api-key.age" ]]; then
    TAILSCALE_API_KEY=$(cd "${NIX_SECRETS_DIR}" && RULES="${NIX_SECRETS_DIR}/secrets.nix" agenix -d tailscale-api-key.age)
  elif [[ -f "${ROOT}/../nix-secrets/tailscale-api-key.age" ]]; then
    TAILSCALE_API_KEY=$(cd "${ROOT}/../nix-secrets" && RULES="${ROOT}/../nix-secrets/secrets.nix" agenix -d tailscale-api-key.age)
  else
    echo "Set TAILSCALE_API_KEY or NIX_SECRETS_DIR (with tailscale-api-key.age)." >&2
    exit 1
  fi
fi

TAILNET=$(tailscale status --json | jq -r '.CurrentTailnet.Name')

HTTP_CODE=$(curl -sS -o /tmp/tailscale-acl-apply.out -w "%{http_code}" \
  -u "${TAILSCALE_API_KEY}:" \
  -H "Content-Type: application/json" \
  --data-binary "@${ACL_FILE}" \
  "https://api.tailscale.com/api/v2/tailnet/${TAILNET}/acl")

if [[ "${HTTP_CODE}" != "200" ]]; then
  echo "Failed to apply ACL (HTTP ${HTTP_CODE})." >&2
  cat /tmp/tailscale-acl-apply.out >&2 || true
  exit 1
fi

echo "Tailscale ACL applied."
