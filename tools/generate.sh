#!/usr/bin/env bash
set -euo pipefail

mkdir -p proto/gen

protoc \
  --go_out=. --go_opt=module=github.com/joshp123/gohome \
  --go-grpc_out=. --go-grpc_opt=module=github.com/joshp123/gohome \
  proto/registry.proto \
  proto/config/v1/config.proto \
  proto/plugins/tado.proto \
  proto/plugins/daikin.proto \
  proto/plugins/growatt.proto \
  proto/plugins/roborock.proto \
  proto/plugins/p1_homewizard.proto \
  proto/plugins/airgradient.proto \
  proto/plugins/weheat.proto
