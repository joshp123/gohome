#!/usr/bin/env bash
set -euo pipefail

mkdir -p proto/gen

protoc \
  --go_out=. --go_opt=module=github.com/elliot-alderson/gohome \
  --go-grpc_out=. --go-grpc_opt=module=github.com/elliot-alderson/gohome \
  proto/registry.proto \
  proto/plugins/tado.proto
