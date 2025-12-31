# RFC: Metrics as Database + Plugin Dashboards

- Date: 2025-12-31
- Status: Implemented
- Audience: plugin authors, operators, observability owners

## 1) Narrative: what we are building and why

GoHome treats metrics as the primary state store. Current device state and
historical trends live in VictoriaMetrics and are visualized in Grafana. Plugins
own their metrics and dashboards so observability is complete the moment a
plugin is enabled.

This RFC defines the observability contract: metric naming, dashboard packaging,
and the plumbing between GoHome, VictoriaMetrics, and Grafana.

## 1.1) Non‑negotiables

- Prometheus metrics are the source of truth (no internal database)
- Grafana is the only UI for visual state
- Plugins ship their own dashboards and metric definitions
- Metrics must be scrapeable from a single /metrics endpoint

## 2) Goals / Non‑goals

Goals:
- Consistent metric naming and labels across plugins
- Central scrape endpoint for all plugin collectors
- Dashboards automatically provisioned via Nix

Non‑goals:
- Alerting and paging in MVP
- Historical query APIs beyond PromQL (future HistoryService)
- Custom UI rendering inside GoHome

## 3) System overview

Plugins register Prometheus collectors with the core. The core exposes a single
/metrics endpoint. VictoriaMetrics scrapes and stores data. Grafana is provisioned
with plugin dashboard JSON files served or staged by GoHome.

## 4) Components and responsibilities

- Plugin collectors: emit device state and events
- Core metrics registry: aggregates collectors into /metrics
- VictoriaMetrics: time-series storage
- Grafana: dashboards + visualization
- Nix provisioning: binds VM + Grafana to GoHome assets

## 5) Inputs / workflow profiles

Inputs:
- Plugin collectors and metric definitions
- Dashboard JSON from plugins
- Nix configuration enabling VM + Grafana

Validation rules:
- Metrics must include unit suffix (e.g., _celsius, _percent, _watts)
- Label cardinality must be bounded (zone, device_id, not timestamps)

### 5.1) Naming conventions (required)

- All metrics are prefixed with `gohome_<plugin_id>_`
- Counters end with `_total`
- Units are explicit: `_celsius`, `_percent`, `_watts`, `_seconds`, `_bool`
- Labels are bounded enums (e.g., `zone`, `device_id`, `mode`)
- No free-form labels (no names, addresses, or timestamps)

## 6) Artifacts / outputs

- /metrics endpoint (Prometheus exposition format)
- VictoriaMetrics TSDB data
- Grafana dashboard JSON provisioned from plugins

Dashboards are served under:
- `/dashboards/<plugin_id>/<dashboard_name>.json`

## 7) State machine (if applicable)

Not applicable. Metrics are continuous streams.

## 8) API surface (protobuf)

No gRPC changes in MVP. Future HistoryService (PromQL proxy) is out of scope
for this RFC.

## 9) Interaction model

- Grafana queries VictoriaMetrics for visualization
- Clawdis and operators use Grafana for state inspection
- gRPC controls device state; metrics reflect outcomes

## 10) System interaction diagram

```
Plugin collectors
   | Prometheus registry
   v
GoHome /metrics
   | scrape
   v
VictoriaMetrics
   | query
   v
Grafana dashboards
```

## 11) API call flow (detailed)

1) VictoriaMetrics scrapes GoHome /metrics on an interval
2) Grafana dashboards query VM with PromQL
3) Users inspect current and historical state in Grafana

## 12) Determinism and validation

- Metric names and labels must be defined in plugin code
- Dashboards are embedded (go:embed) and required at build time
- Nix provisioning pins Grafana + VM versions
- Core exposes `gohome_build_info` (version, commit) for traceability

## 13) Outputs and materialization

Primary outputs:
- Prometheus metrics
- Grafana dashboards (JSON)

No alternate formats required in MVP.

## 14) Testing philosophy and trust

- Unit tests for collector output formatting
- Integration test with VictoriaMetrics scrape (optional)
- Dashboard JSON linting (optional) before provisioning

## 15) Incremental delivery plan

1) Expose /metrics with a core build_info metric
2) Add a plugin with real device metrics (Tado)
3) Provision Grafana with plugin dashboard

## 16) Implementation order

1) Implement metrics registry in core
2) Add plugin collector interface and wiring
3) Add dashboard serving/packing
4) Configure VM + Grafana in Nix

## 16.1) Dependencies / sequencing

- Dependencies: core/plugin contract; Nix-native build (for wiring services)
- Blocks: meaningful Grafana UI and metrics visibility
- Next: add core metrics registry + build_info

## 17) Brutal self‑review (required)

- Junior engineer: Are naming rules explicit enough? If not, add a short
  naming guide in a plugin README. This RFC now includes mandatory rules.
- Mid‑level engineer: Are we ignoring label cardinality risks? The rules
  need to be enforced in code review.
- Senior/principal engineer: Does “metrics as DB” block future stateful
  features? It is intentional for MVP; a dedicated store can be added later.
- PM: Does this give users visibility without building UI? Yes.
- EM: Is operational burden reasonable? VM + Grafana are proven and Nix-managed.
- External stakeholder: Is data retention safe? VM retention is long by design.
- End user: Will state be visible and trustworthy? Grafana + VM provide that.
