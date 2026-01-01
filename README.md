# GoHome

> Home automation for people who hate home automation software.

## What is this?

GoHome is a **Nix‑native** home automation server.

- Config via NixOS modules (no YAML, no env vars)
- Control via gRPC + CLI (proto/registry discovery)
- State via VictoriaMetrics
- Dashboards via Grafana
- Plugins compiled in at build time

## Status

Pre‑alpha. Building in public.

## Quick Start (NixOS)

```nix
# flake.nix
{
  inputs.gohome.url = "github:joshp123/gohome";

  outputs = { self, nixpkgs, gohome }: {
    nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
      modules = [
        gohome.nixosModules.default
        {
          services.gohome = {
            enable = true;
            oauth = {
              blobEndpoint = "https://s3.eu-central-1.amazonaws.com";
              blobBucket = "gohome-oauth-homelab-eu-central-1";
              blobAccessKeyFile = config.age.secrets.gohome-oauth-blob-access-key.path;
              blobSecretKeyFile = config.age.secrets.gohome-oauth-blob-secret-key.path;
            };
            plugins.tado = {
              bootstrapFile = config.age.secrets.tado-token.path;
            };
          };
        }
      ];
    };
  };
}
```

## Development

```bash
# Generate protobufs (requires nix)
nix develop -c ./tools/generate.sh

# Run server
go run -tags gohome_plugin_tado ./cmd/gohome

# Discover via CLI
go run ./cmd/gohome-cli plugins list
```

## Architecture (MVP)

- Core router + Registry (gRPC discovery)
- Plugins own proto, metrics, dashboards, and AGENTS.md
- Observability via Prometheus/VictoriaMetrics + Grafana

## Docs

- `PHILOSOPHY.md`
- `docs/rfc/INDEX.md`

## License

AGPL‑3.0.
