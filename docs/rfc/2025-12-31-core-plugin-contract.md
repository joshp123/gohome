# RFC: Core Router + Compile-Time Plugin Contract

- Date: 2025-12-31
- Status: Implemented
- Audience: engineers, operators, future plugin authors

## 1) Narrative: what we are building and why

GoHome exists to replace Home Assistant with a **Nix-native, Go-first** home automation
system that is deterministic, observable, and agent-friendly. The core product is
**a router** that exposes plugin APIs over gRPC and aggregates their metrics and
assets, while **plugins own all domain behavior** (schemas, dashboards, and device
clients). This keeps the core small, reduces coupling, and ensures every plugin can
be reasoned about independently by AI agents.

This RFC locks the core/plugin contract so the rest of the system can evolve without
breaking the integration surface.

## 1.1) Non‑negotiables

- Nix-native configuration (no runtime config editing)
- API-first (protobuf/gRPC is the product)
- ZFC-compliant: AI decides; GoHome executes mechanically
- Plugins compiled into the binary (no runtime loading for MVP)
- Plugin assets (AGENTS.md, dashboards) must be compile-time required
- No UI in core (Grafana is the UI)

## 2) Goals / Non‑goals

Goals:
- A stable compile-time plugin interface for routing, discovery, and assets
- A Registry service for discovery and agent context (AGENTS.md)
- A single gRPC server hosting core + plugin services
- Predictable plugin health reporting (HEALTHY/DEGRADED/ERROR)

Non‑goals:
- Runtime plugin loading
- Automation engine / event bus
- UI rendering beyond serving static dashboards
- Cross-plugin orchestration semantics

## 3) System overview

GoHome core links a set of plugins at build time, registers their gRPC services,
exports metrics on a single /metrics endpoint, and serves plugin dashboards and
agent context. Clawdis (or any gRPC client) discovers capabilities via the Registry
API and gRPC reflection, then calls plugin APIs directly.

## 4) Components and responsibilities

- Core router: hosts gRPC server, registry service, health aggregation
- Plugin contract: interface implemented by each plugin
- Registry service: discovery + metadata + AGENTS.md distribution
- Metrics aggregation: exports plugin collectors on a single /metrics endpoint
- Dashboard server: serves plugin Grafana JSON bundles

### 4.1) Plugin interface (Go)

Minimum interface (compile-time contract):

```go
type Plugin interface {
    ID() string                         // stable, lowercase, snake case
    Manifest() Manifest                 // name, version, services
    AgentsMD() string                   // go:embed, required
    Dashboards() []Dashboard            // go:embed, required
    RegisterGRPC(*grpc.Server)
    Collectors() []prometheus.Collector
    Health() HealthStatus               // HEALTHY/DEGRADED/ERROR
}

type Manifest struct {
    PluginID    string
    DisplayName string
    Version     string
    Services    []string                // fully-qualified gRPC service names
}

type Dashboard struct {
    Name string                          // e.g., "tado-overview"
    JSON []byte
}

type HealthStatus string

const (
    HealthHealthy  HealthStatus = "HEALTHY"
    HealthDegraded HealthStatus = "DEGRADED"
    HealthError    HealthStatus = "ERROR"
)
```

ID rules:
- `plugin_id` is stable and matches `^[a-z][a-z0-9_]+$`
- Folder name must match `plugin_id`
- Metrics must use the same prefix (`gohome_<plugin_id>_...`)

## 5) Inputs / workflow profiles

Inputs:
- Build-time plugin set (Nix configuration decides which plugins are compiled)
- Plugin manifests (static metadata)
- Plugin assets (AGENTS.md, dashboards) embedded via go:embed

Validation rules:
- Missing AGENTS.md or dashboard JSON must fail the build
- All plugins must register their gRPC services during startup

## 6) Artifacts / outputs

- Single GoHome binary with compiled-in plugins
- Registry metadata (including full AGENTS.md content)
- gRPC services for core + plugins
- /metrics endpoint for Prometheus/VictoriaMetrics
- /dashboards endpoint for Grafana JSON

## 7) State machine (if applicable)

Not applicable. Core is a stateless router; plugin state is external (device APIs,
metrics time-series).

## 8) API surface (protobuf)

Core proto (proto/registry.proto):
- Registry.ListPlugins
- Registry.DescribePlugin
- Registry.WatchPlugins

Required plugin descriptor fields:
- plugin_id, display_name, version
- services (fully-qualified gRPC service names)
- agents_md (full text)
- dashboards (names/paths)
- status (HEALTHY/DEGRADED/ERROR)
 - health_message (optional short reason when degraded/error)

Plugin protos live under proto/plugins/<name>.proto and define all device actions.

## 9) Interaction model

- Clawdis connects over gRPC
- Reads Registry + gRPC reflection
- Uses AGENTS.md for capability and limits
- Calls plugin RPCs directly
- Observes state via Prometheus metrics and Grafana

### 9.1) Progressive CLI discovery (Clawdis-friendly)

We provide a proto‑generated CLI that uses Registry + reflection to discover
available plugins, services, and methods at runtime. This CLI is the primary
interface for new agents and new plugins, and it is the basis for Clawdis to
self‑discover capability without prior manual wiring.

Known workflows can be “crystallized” into a Clawdis skill that simply shells
out to the CLI (symlinked from the repo). Unknown workflows use discovery.

## 10) System interaction diagram

```
Clawdis/CLI
   | ListPlugins / DescribePlugin
   v
GoHome Core (Registry + Router)
   | Plugin RPC
   v
Plugin (Tado, Daikin, ...)
   | Device API
   v
External Service
```

## 11) API call flow (detailed)

1) Client calls Registry.ListPlugins
2) Client calls Registry.DescribePlugin for a target plugin
3) Client inspects AGENTS.md + gRPC reflection
4) Client calls plugin RPC (e.g., SetTemperature)
5) Plugin executes device API call and returns response
6) Metrics are exported continuously via /metrics

CLI path:
1) CLI queries Registry and reflection
2) CLI materializes commands for each plugin/service
3) CLI executes gRPC calls with typed arguments

## 12) Determinism and validation

- go:embed enforces required assets at compile time
- Core refuses to start if any plugin fails registration
- Registry output is deterministic from compiled plugin set
 - Plugin ID rules are validated at startup (fail fast)

## 13) Outputs and materialization

Primary outputs:
- Protobuf RPCs for control
- Prometheus metrics for state
- Grafana dashboards for visualization

No alternate materializations are required in MVP.

## 14) Testing philosophy and trust

- Unit tests for plugin registration and registry output
- Integration test with one plugin (Tado) and a mock API server
- Compile-time asset tests (go:embed ensures existence)

## 15) Incremental delivery plan

1) Core router + Registry with a stub plugin
2) Embed AGENTS.md + dashboard assets
3) Add one real plugin (Tado)

## 16) Implementation order

1) Define plugin interface + manifest
2) Implement registry.proto and core registry service
3) Wire gRPC server + reflection
4) Add metrics + dashboard serving
5) Add Tado plugin against this contract

## 16.1) Dependencies / sequencing

- Dependencies: PHILOSOPHY.md (non‑negotiables)
- Blocks: all plugin implementations; Nix build composition
- Next: implement core router + registry + reflection

## 17) Brutal self‑review (required)

- Junior engineer: Is the plugin interface concrete enough to implement? Yes,
  a minimal Go interface and structs are defined above.
- Mid‑level engineer: Are failure modes defined (plugin init errors, partial health)?
  Yes, health status semantics are explicit and required.
- Senior/principal engineer: Does this lock us out of runtime loading later?
  It shouldn’t; compile-time is the MVP default, not a permanent constraint.
- PM: Does this enable the MVP scope (Tado + metrics + CLI)? Yes.
- EM: Are the boundaries clean for parallel work? Core and plugins can be developed
  independently once the contract is fixed.
- External stakeholder: Is the security story clear? This RFC does not define auth;
  see the deployment/security RFC.
- End user: Will this feel reliable? Determinism + compile-time assets minimize
  runtime surprises.
