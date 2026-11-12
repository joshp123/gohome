# ADR: Dashboards are not part of gohome

Date: 2026-11-12
Status: Accepted

Written by AI on 2026.11.12 by model claude opus 5.5.

## Context

Plugins shipped Grafana dashboards: an embedded `dashboard.json` each, and a `home`
plugin whose only job was to carry the whole-house views. A dashboard describes one
house: its rooms, its devices, its contracts. So the product carried one deployment's
views, and every change to a view was a change to the product.

## Decision

gohome reads and controls devices and publishes readings, controls and health through
typed interfaces: gRPC, the registry and Prometheus metrics. It ships no dashboards.

Its NixOS module still runs VictoriaMetrics and Grafana and provisions the datasource.
A deployment provisions its own dashboards with NixOS's
`services.grafana.provision.dashboards` and sets Grafana's home page itself.

Removed: `Plugin.Dashboards()`, the `/dashboards/` HTTP route, the registry's dashboard
list (field 6, reserved), `core.dashboard_dir` (field 3, reserved), the `home` plugin
(config field 17, reserved) and the dashboard generator.

## Consequences

- A new deployment gets Grafana with the datasource and no dashboards.
- What dashboards rely on is the metrics (names, labels, units) and the plugins' HTTP
  routes, such as camera pictures and vacuum maps. Changing either breaks dashboards
  that live outside this repo.
- Supersedes the dashboard parts of the core plugin contract and metrics RFCs of
  2025-12-31.
