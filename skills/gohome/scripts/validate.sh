#!/usr/bin/env bash
set -euo pipefail

GOHOME_HOST="${GOHOME_HOST:-gohome}"
GOHOME_HTTP_BASE="${GOHOME_HTTP_BASE:-http://${GOHOME_HOST}}"
GOHOME_GRPC_ADDR="${GOHOME_GRPC_ADDR:-${GOHOME_HOST}:9000}"

run_cli() {
  if command -v gohome-cli >/dev/null 2>&1; then
    GOHOME_GRPC_ADDR="${GOHOME_GRPC_ADDR}" gohome-cli "$@"
    return
  fi
  if [ -x "./bin/gohome-cli" ]; then
    GOHOME_GRPC_ADDR="${GOHOME_GRPC_ADDR}" ./bin/gohome-cli "$@"
    return
  fi
  if [ -x "./result/bin/gohome-cli" ]; then
    GOHOME_GRPC_ADDR="${GOHOME_GRPC_ADDR}" ./result/bin/gohome-cli "$@"
    return
  fi
  GOHOME_GRPC_ADDR="${GOHOME_GRPC_ADDR}" go run ./cmd/gohome-cli "$@"
}

echo "Target: ${GOHOME_HOST}"
run_cli plugins list
run_cli plugins describe tado
run_cli methods gohome.plugins.tado.v1.TadoService
run_cli call gohome.plugins.tado.v1.TadoService/ListZones --data '{}'

curl -s "${GOHOME_HTTP_BASE}/gohome/metrics" | grep -E "gohome_tado_" | head -n 20

echo "OK"
