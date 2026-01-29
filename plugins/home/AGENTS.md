# My Home Overview - Agent Context

## What This Is
A read-only, cross-system Grafana overview for the whole house. This plugin has **no device APIs** and only ships a dashboard.

## Purpose
- Single "My Home" dashboard with comfort-first layout.
- Pulls metrics from other plugins (Tado, Weheat, Daikin, AirGradient, P1 Homewizard, Growatt, Roborock).

## Capabilities
- Dashboard only (`my-home-overview`)
- No gRPC services
- No metrics of its own

## Required Config
Enable the plugin via Nix:
```
services.gohome.plugins.home = {};
```

## Notes
- Living room + bedroom comfort from Tado sensors.
- Heating supply from Weheat; setpoint from Daikin.
- Air quality from AirGradient (bedroom sensor).
- Energy from P1 Homewizard + Growatt.
