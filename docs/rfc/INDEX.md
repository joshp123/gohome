# GoHome RFC Index (DAG)

This index shows the dependency order for implementing the RFC set.

## Nodes

- PHILOSOPHY.md (non‑negotiables)
- RFC: Core router + compile‑time plugin contract
- RFC: Nix‑native configuration and build composition
- RFC: Nix‑native typed config handoff (Go)
- RFC: Metrics as database + dashboards
- RFC: Deployment + security boundary
- RFC: Reverse proxy + index landing page
- RFC: OAuth credentials and token rotation

## DAG (edges)

```
PHILOSOPHY.md
  -> Core/plugin contract
  -> Nix‑native config/build

Core/plugin contract
  -> Metrics + dashboards
  -> Nix‑native config/build
  -> OAuth credentials + token rotation

Nix‑native config/build
  -> Nix‑native typed config handoff
  -> Deployment + security boundary
  -> Reverse proxy + index
  -> OAuth credentials + token rotation

Metrics + dashboards
  -> Reverse proxy + index (for Grafana subpath convenience)
```

## Suggested implementation order

1) Core/plugin contract
2) Nix‑native config/build
3) Nix‑native typed config handoff
4) Metrics + dashboards
5) OAuth credentials + token rotation
6) Deployment + security boundary
7) Reverse proxy + index
