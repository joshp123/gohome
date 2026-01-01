---
name: gohome-clawdis
description: Use when Clawdis needs to test or operate GoHome (Home Assistant clone) via gRPC discovery, metrics, and Grafana.
---

# GoHome Clawdis Skill

This skill teaches Clawdis how to discover capabilities and validate the GoHome stack
using the repo CLI, Prometheus metrics, and Grafana.

## Quick start

Set the target host and ports:

```sh
export GOHOME_HOST="gohome"
export GOHOME_HTTP_BASE="http://${GOHOME_HOST}"
export GOHOME_GRPC_ADDR="${GOHOME_HOST}:9000"
```

Use the repo-shipped CLI binary (preferred):

```sh
./bin/gohome-cli services
```

Use the CLI from source if needed:

```sh
go run ./cmd/gohome-cli services
```

If Go is not available, build with Nix and run the CLI from the result:

```sh
nix build .#packages.x86_64-linux.default
./result/bin/gohome-cli services
```

## Discovery flow (read-only)

1) List plugins:

```sh
GOHOME_GRPC_ADDR="$GOHOME_GRPC_ADDR" go run ./cmd/gohome-cli plugins list
```

2) Describe a plugin:

```sh
GOHOME_GRPC_ADDR="$GOHOME_GRPC_ADDR" go run ./cmd/gohome-cli plugins describe tado
```

3) List methods for a service:

```sh
GOHOME_GRPC_ADDR="$GOHOME_GRPC_ADDR" go run ./cmd/gohome-cli methods gohome.plugins.tado.v1.TadoService
```

4) Call a safe RPC (read-only):

```sh
GOHOME_GRPC_ADDR="$GOHOME_GRPC_ADDR" go run ./cmd/gohome-cli call gohome.plugins.tado.v1.TadoService/ListZones --data '{}'
```

## Metrics validation

Confirm the Tado scraper is healthy and metrics are present:

```sh
curl -s "${GOHOME_HTTP_BASE}/gohome/metrics" | rg -n "gohome_tado_"
```

Expect:
- `gohome_tado_scrape_success 1`
- zone temperature + humidity metrics

## Grafana access

Grafana is proxied under:

```
${GOHOME_HTTP_BASE}/grafana/
```

Use MagicDNS (`gohome`) or set `GOHOME_HOST` to the tailnet FQDN if needed.

## Stateful / destructive actions (require explicit approval)

Only call write RPCs after user approval. Example:

```sh
GOHOME_GRPC_ADDR="$GOHOME_GRPC_ADDR" go run ./cmd/gohome-cli call \
  gohome.plugins.tado.v1.TadoService/SetTemperature \
  --data '{"zone_id":"1","temperature_celsius":20.0}'
```

## Troubleshooting

- If DNS fails, verify MagicDNS is enabled and run `tailscale status`.
- If metrics are missing, check `gohome_tado_scrape_success` and token validity.
