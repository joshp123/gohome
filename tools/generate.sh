#!/usr/bin/env bash
set -euo pipefail

mkdir -p proto/gen

protoc \
  --go_out=. --go_opt=module=github.com/joshp123/gohome \
  --go-grpc_out=. --go-grpc_opt=module=github.com/joshp123/gohome \
  proto/registry.proto \
  proto/plugins/tado.proto \
  proto/plugins/daikin.proto
