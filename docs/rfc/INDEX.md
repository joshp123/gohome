# GoHome RFC Index (DAG)

This index shows the dependency order for implementing the RFC set.

## Nodes

- PHILOSOPHY.md (non-negotiables)
- RFC: Core router + compile-time plugin contract
- RFC: Nix-native configuration and build composition
- RFC: Nix-native typed config handoff (Go)
- RFC: Metrics as database + dashboards
- RFC: Deployment + security boundary
- RFC: Reverse proxy + index landing page
- RFC: OAuth credentials and token rotation
- RFC: Core rate limits (plugin-declared, core-enforced)
- RFC: Integration research - P1 Monitor (P1 Meter)
- RFC: Integration research - UNii Alarm System
- RFC: Integration research - Growatt
- RFC: Integration research - Roborock
- RFC: Integration research - Meaco

## DAG (edges)

```
PHILOSOPHY.md
  -> Core/plugin contract
  -> Nix-native config/build

Core/plugin contract
  -> Metrics + dashboards
  -> Nix-native config/build
  -> OAuth credentials + token rotation
  -> Core rate limits
  -> Integration research: P1 Monitor, Growatt, Roborock, Meaco, UNii

Nix-native config/build
  -> Nix-native typed config handoff
  -> Deployment + security boundary
  -> Reverse proxy + index
  -> OAuth credentials + token rotation
  -> Integration research: P1 Monitor, Growatt, Roborock, Meaco, UNii

Metrics + dashboards
  -> Reverse proxy + index (for Grafana subpath convenience)
  -> Integration research: P1 Monitor, Growatt, Roborock, Meaco, UNii

OAuth credentials + token rotation
  -> Integration research: Growatt, Roborock
```

## Suggested implementation order

1) Core/plugin contract
2) Nix-native config/build
3) Nix-native typed config handoff
4) Metrics + dashboards
5) OAuth credentials + token rotation
6) Core rate limits
7) Deployment + security boundary
8) Reverse proxy + index

## Suggested integration order (easiest -> hardest)

1) P1 Monitor (P1 Meter)
2) Growatt
3) Roborock
4) Meaco
5) UNii Alarm System

Notes:
- UNii is bespoke; treat as discovery-first.
