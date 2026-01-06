---
name: gohome
description: Use when Clawdbot needs to test or operate GoHome (Home Assistant clone) via gRPC discovery, metrics, and Grafana.
---

# GoHome Skill

This skill teaches Clawdbot how to discover capabilities and validate the GoHome stack
using the repo CLI, Prometheus metrics, and Grafana.

## Quick start

Set the target host and ports (optional; defaults read from config or MagicDNS):

```sh
export GOHOME_HOST="gohome"
export GOHOME_HTTP_BASE="http://${GOHOME_HOST}:8080"
export GOHOME_GRPC_ADDR="${GOHOME_HOST}:9000"
```

Use the CLI on PATH (preferred, installed by the nix-clawdbot plugin):

```sh
gohome-cli services
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
jq -n '{}' | GOHOME_GRPC_ADDR="$GOHOME_GRPC_ADDR" go run ./cmd/gohome-cli call \
  gohome.plugins.tado.v1.TadoService/ListZones
```

## Friendly CLI (agent-friendly)

Roborock:

```sh
gohome-cli roborock status
gohome-cli roborock rooms
gohome-cli roborock clean kitchen
gohome-cli roborock mop kitchen
gohome-cli roborock vacuum kitchen
gohome-cli roborock smart kitchen
gohome-cli roborock map --labels names
```

## Sending maps to users (Telegram/messaging)

When user asks for a map, send it as an image they can see inline. Use the MEDIA: syntax with the gohome map URL:

```
MEDIA:http://gohome:8080/roborock/map.png?device_name=Roborock+Qrevo+S&labels=names

Robot on dock in hallway. Battery 100%.
```

This sends the map image directly in the chat. Options:
- `labels=names` — room names (recommended)
- `labels=segments` — segment IDs
- no labels param — clean map

Do NOT just describe the map or show it in a code block — send it as a MEDIA: image so users can see it.

Tado:

```sh
gohome-cli tado zones
gohome-cli tado set living-room 20
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
jq -n --arg zone_id "1" --argjson temp 20.0 \
  '{zone_id: $zone_id, temperature_celsius: $temp}' | \
  GOHOME_GRPC_ADDR="$GOHOME_GRPC_ADDR" go run ./cmd/gohome-cli call \
  gohome.plugins.tado.v1.TadoService/SetTemperature
```

## Troubleshooting

- If DNS fails, verify MagicDNS is enabled and run `tailscale status`.
- If metrics are missing, check `gohome_tado_scrape_success` and token validity.
- Prefer `jq -n` to build JSON for gRPC calls; it avoids quoting mistakes.
- `gohome-cli` reads `/etc/gohome/config.pbtxt` (or `~/.config/gohome/config.pbtxt`) for default host info.
