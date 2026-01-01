# GoHome Philosophy

> ⚠️ **SPEC DOCUMENT** - This is the plan, not the implementation.

> **Home automation for people who hate home automation software.**

## Why GoHome Exists

Home Assistant is powerful but wrong:
- **Python** - slow, memory hungry, asyncio hell
- **YAML config** - runtime editable, drifts, breaks
- **Lovelace UI** - yet another thing to maintain
- **Add-ons/HACS** - dependency hell, version conflicts
- **SQLite recorder** - wrong tool for time-series

We want:
- **Go** - fast, single binary, boring
- **Nix config** - declarative, reproducible, rollback
- **Grafana** - already good, don't reinvent
- **VictoriaMetrics** - already good, don't reinvent
- **Clawdis/API** - control surface, not dashboards

## Core Principles

### 1. Nix-Native, Not Nix-Compatible

```nix
# This IS your config. Not a wrapper around YAML.
services.gohome.plugins.tado = {
  enable = true;
  passwordFile = config.age.secrets.tado.path;
};
```

- Config changes require `nixos-rebuild`
- No runtime config editing
- Secrets via agenix/sops, never plaintext
- Rollback is `nixos-rebuild switch --rollback`

### 2. Core is a Router, Plugins Own Everything

Core does:
- Load plugins
- Route gRPC calls
- Aggregate metrics
- Serve dashboards

Core does NOT:
- Know about Tado, Roborock, etc.
- Define device schemas
- Render UIs
- Store state (that's VictoriaMetrics)

Plugins own:
- Their `AGENTS.md` (AI context - REQUIRED, first-class)
- Their `.proto` API definition
- Their Prometheus metrics
- Their Grafana dashboards
- Their Nix module

### 3. Prometheus IS the Database

No SQLite. No PostgreSQL. No "recorder" component.

```
State change → Prometheus metric → Query with PromQL
```

Current state? Query latest value.
History? Query range.
Aggregations? PromQL.

VictoriaMetrics handles compression, retention, clustering.

### 4. Plugins Are Compiled In

Plugins are compiled into the binary (not runtime loaded):

```nix
# Nix composes the binary with enabled plugins
packages.gohome = gohome.override {
  plugins = [ tado daikin ];
};
```

- Compile-time plugin loading (simple, type-safe)
- Nix handles composition
- No runtime plugin discovery complexity
- Add plugin = rebuild binary
- **Single‑instance only**: multi‑instance deployments are unsupported.

### 5. API-First, UI-Never

We don't build UIs. Ever.

- **Dashboards** → Grafana (plugins provide JSON)
- **Control** → Clawdis (Telegram, voice)
- **Debugging** → grpcurl, curl
- **Automation** → Nix config, not UI flows

### 6. If It Compiles, It Works

No runtime surprises. The type system enforces the plugin contract.

```go
type Plugin interface {
    Manifest() Manifest
    AgentsMD() string              // go:embed - missing file = compile fail
    RegisterGRPC(*grpc.Server)
    Collectors() []prometheus.Collector
    Dashboards() []Dashboard       // go:embed - missing file = compile fail
    // ...
}
```

- Missing `AGENTS.md`? **Compile error.**
- Missing `dashboard.json`? **Compile error.**
- Wrong proto? **Compile error.**
- Nix validates config schema at build time.

Runtime only does mechanical execution. All validation at compile time.

### 7. Agents Build This

This repo is designed to be built BY AI agents, FOR AI agents.

- Clear module boundaries
- Protobuf contracts (typed, validated)
- Nix for reproducibility (no "works on my machine")
- Everything greppable
- If it compiles, it works

## Agent Interaction Model (ZFC-Compliant)

### How Clawdis Talks to GoHome

```
User: "set living room to 21"
         │
         ▼
    ┌─────────┐
    │ Clawdis │  
    │  (AI)   │  ← AI reasons about intent + available methods
    └────┬────┘
         │ gRPC (AI-decided call)
         ▼
    ┌─────────┐
    │ GoHome  │  ← Pure routing, no logic
    │  Core   │
    └────┬────┘
         │
         ▼
    ┌─────────┐
    │  Tado   │  ← Mechanical execution
    │ Plugin  │
    └─────────┘
```

### Plugin Discovery (Dynamic)

```protobuf
service Registry {
  rpc ListPlugins(...) returns (PluginList);
  rpc DescribePlugin(...) returns (PluginDescriptor);
  rpc WatchPlugins(...) returns (stream PluginEvent);
}
```

Clawdis (ZFC flow):
1. Connects to GoHome
2. Fetches available services/methods (gRPC reflection)
3. On user message: AI decides which method to call
4. Execute mechanically

### Natural Language Routing (ZFC-Compliant)

**NO pattern matching. NO keyword routing. AI does all reasoning.**

Plugins expose their protobuf schema. Clawdis sends:
1. User message
2. Available services/methods (from Registry)
3. Current device state (optional context)

AI decides which gRPC method to call. Execute mechanically.

```go
// WRONG (ZFC violation) ❌
func (p *TadoPlugin) NLTriggers() map[string][]string {
    return map[string][]string{
        "SetTemperature": {"set {zone} to {temp}", "make it warmer"},
    }
}

// RIGHT (ZFC compliant) ✅
// Plugin just exposes schema via gRPC reflection
// Clawdis asks AI: "Given this user message and these available methods, what should I call?"
// AI returns: {service: "TadoService", method: "SetTemperature", params: {zone: "living_room", temp: 21}}
// Execute mechanically
```


## Porting HA Plugins

Home Assistant has 1395 integrations. We want them.

### Porting Strategy

1. **Identify API** - Most HA integrations wrap a REST/MQTT/local API
2. **Find the client library** - Often a Python lib we can replace
3. **Write Go client** - Or use existing Go lib
4. **Define protobuf** - API surface for Clawdis
5. **Define metrics** - What to export to Prometheus
6. **Create dashboard** - Grafana JSON
7. **Write Nix module** - Config schema, secrets

### What to Port First

Priority based on:
- Your devices (e.g. thermostats, vacuums, heat pumps)
- API quality (REST > cloud-only > Bluetooth)
- Community demand

### Porting Checklist

```markdown
## Plugin: {name}

- [ ] AGENTS.md (REQUIRED - AI context, first-class contract)
- [ ] API research (endpoints, auth, rate limits)
- [ ] Go client (or wrap existing)
- [ ] proto/{name}.proto (gRPC service definition)
- [ ] metrics.go (Prometheus collectors)
- [ ] dashboard.json (Grafana)
- [ ] nix/module.nix (config schema)
- [ ] Tests
- [ ] README with setup instructions
```

### AGENTS.md Contract (Required)

Every plugin MUST include `AGENTS.md`. Served via Registry API.

```protobuf
message PluginDescriptor {
  string agents_md = 10;  // Full AGENTS.md content
}
```

**Required sections:**

```markdown
# {Plugin} - Agent Context

## What This Is
What devices/services this controls. One paragraph.

## Capabilities  
What the AI can do via this plugin.

## Limits
Constraints, ranges, things that CAN'T be done.

## Methods
Each gRPC method: name, purpose, params, returns.

## State
Metrics exposed. How to query status.

## Errors
Common failures and what they mean.
```

AI agents consume this alongside protobuf schema for full context.

### Example: Porting Tado

HA source: `homeassistant/components/tado/`

**Existing assets to reuse:**

**Porting steps:**
1. **API**: REST, OAuth2 - token already in agenix
2. **Go client**: Port from `python-tado` or generate from OpenAPI spec
3. **Protobuf**: `TadoService` with `GetZones`, `SetTemperature`, `Boost`, etc.
4. **Metrics**: `gohome_tado_temperature_celsius`, `gohome_tado_humidity_percent`, etc.
5. **Dashboard**: Adapt existing Grafana dashboard
6. **Nix**: `plugins.tado.enable`, `plugins.tado.bootstrapFile`

### Example: Porting Daikin

**Existing assets:**
- Fork with fixes: `github.com/sample-org/daikin_onecta` (branch `homelab/fan-direction`)

**Porting steps:**
1. **API**: Daikin Onecta cloud API - credentials in agenix
2. **Go client**: Port from Python integration
3. **Protobuf**: `DaikinService` with `GetUnits`, `SetTemperature`, `SetMode`, etc.
4. **Metrics**: `gohome_daikin_temperature_celsius`, `gohome_daikin_mode`, etc.
5. **Nix**: `plugins.daikin.enable`, `plugins.daikin.bootstrapFile`

## Why We're Better Than Home Assistant

| Aspect | Home Assistant | GoHome |
|--------|----------------|--------|
| Language | Python (slow, asyncio) | Go (fast, simple) |
| Config | YAML (runtime, mutable) | Nix (declarative, immutable) |
| Storage | SQLite (wrong tool) | VictoriaMetrics (right tool) |
| UI | Lovelace (maintain it yourself) | Grafana (already good) |
| Control | HA app, web UI | Clawdis, API, grpcurl |
| Plugins | HACS, pip, Docker | Nix flakes |
| Secrets | YAML plaintext | agenix/sops |
| Rollback | Hope you have a backup | `nixos-rebuild --rollback` |
| Updates | Pray nothing breaks | Nix pins versions |
| RAM | 1-2GB typical | ~256MB |
| Deploy | Docker, HAOS, Supervised | `tofu apply && nixos-rebuild` |

## Who This Is For

- **NixOS users** who want home automation
- **Developers** who hate YAML
- **AI agent builders** who want typed APIs
- **Ops people** who want observability baked in
- **Anyone** who's been burned by HA updates

## Who This Is NOT For

- People who want a pretty UI
- People who can't run `nixos-rebuild`
- People who want to configure via phone app
- People who need Zigbee/Z-Wave coordinator (use zigbee2mqtt, we integrate)

## License

**AGPL-3.0** - Copyleft. If you modify and run it as a service, share your changes.

Why AGPL:
- Prevents cloud providers from taking without giving back
- Ensures community improvements stay in community
- Plugins can be any license (they're in plugins/ directory)

## Project Structure

```
gohome/
├── flake.nix                 # Nix flake (core + module)
├── LICENSE                   # AGPL-3.0
├── PHILOSOPHY.md             # This document
├── README.md
├── go.mod
├── cmd/
│   └── gohome/
│       └── main.go
├── internal/
│   ├── core/                 # State, events, plugin loader
│   ├── router/               # gRPC routing
│   ├── metrics/              # Prometheus aggregation
│   └── dashboard/            # Grafana serving
├── proto/
│   └── registry.proto        # Core service discovery
├── nix/
│   ├── package.nix           # Go derivation
│   └── module.nix            # NixOS module
└── infra/
    ├── tofu/                 # OpenTofu for VPS
    └── examples/             # Example deployments
```

Plugins live in `plugins/` directory (monorepo for simplicity):
```
plugins/
├── tado/
├── daikin/
├── roborock/
...
```

Can split to separate repos later if needed.

## Contributing

### For Humans

1. Pick a plugin to port
2. Follow the porting checklist
3. Open PR with tests
4. Nix must build, `nix flake check` must pass

### For AI Agents

1. Read this document
2. Read target plugin's HA source
3. Research the device's API
4. Generate protobuf first (contract-first design)
5. Implement, test, document

Clear boundaries. Typed contracts. Reproducible builds. Agent-friendly.

---

*Built with contempt for YAML and love for Nix.* 🏠
