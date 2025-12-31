# GoHome RFC Index (DAG)

This index shows the dependency order for implementing the RFC set.

## Nodes

- PHILOSOPHY.md (non‑negotiables)
- RFC: Core router + compile‑time plugin contract
- RFC: Nix‑native configuration and build composition
- RFC: Metrics as database + dashboards
- RFC: Deployment + security boundary
- RFC: Reverse proxy + index landing page

## DAG (edges)

```
PHILOSOPHY.md
  -> Core/plugin contract
  -> Nix‑native config/build

Core/plugin contract
  -> Metrics + dashboards
  -> Nix‑native config/build

Nix‑native config/build
  -> Deployment + security boundary
  -> Reverse proxy + index

Metrics + dashboards
  -> Reverse proxy + index (for Grafana subpath convenience)
```

## Suggested implementation order

1) Core/plugin contract
2) Nix‑native config/build
3) Metrics + dashboards
4) Deployment + security boundary
5) Reverse proxy + index
