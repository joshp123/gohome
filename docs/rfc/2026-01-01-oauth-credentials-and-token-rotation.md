# RFC: OAuth Credentials and Token Rotation (Minimal)

- Date: 2026-01-01
- Status: Draft
- Audience: engineers, operators, plugin authors, agents
- Owner: GoHome core

## 0) Philosophy (applies to this RFC)

- **Nix‑native, declarative**: configuration is code; no runtime config edits.
  Runtime state file writes are expected.
- **Mutable runtime state is intentional**: the state file is the source of truth
  for refresh tokens after bootstrap; Nix seeds but does not overwrite it.
- **If it compiles, it works**: interface presence is compile‑time enforced.
- **Core is mechanical**: plugins declare endpoints; core executes.
- **One obvious way**: a single schema and deterministic persistence flow.
- **Single‑instance only**: multi‑instance deployments are explicitly unsupported.

## 1) Narrative

GoHome integrates with cloud APIs (Daikin, Tado, etc.) that use OAuth. We need a
small, standard OAuth layer so plugins stop re‑implementing token parsing and
refresh logic. This RFC defines the minimal contract required for headless
refresh + blob recovery.

## 2) Must‑haves

- **Blob mirror always on** (no opt‑out).
- **Deterministic load/write order**:
  - Load: local → blob → bootstrap
  - Write: local → blob
- **Single‑instance only** (explicitly unsupported multi‑instance).
- **Core OAuth runner CLI** is the only auth entry point.
- **Plugin‑declared endpoints/scope only** (no inferred defaults).
- **Minimal state schema** (schema_version, client_id, client_secret, refresh_token, scope).
- **Bootstrap precedence**:
  - client_id/secret from Nix
  - refresh_token from state
  - on successful refresh, write client_id/secret into state
- **Compile‑time enforcement**: plugins must implement the OAuth declaration
  interface (interface presence only).

## 3) Components

- **Core OAuth runner (CLI)**: executes auth‑code/device flows and writes state.
- **Provider declarations**: endpoints, scope, flow (declared by plugins).
- **Token store**: local state file + blob mirror.
- **Token refresher**: refresh + persist rotation.

Runner discovery:
- The runner is built with the same Nix composition as the core binary (same
  plugin set). It loads the compiled plugin registry and calls
  `OAuthDeclaration()` on each plugin.

## 4) State schema (canonical JSON)

```
{
  "schema_version": 1,
  "client_id": "...",
  "client_secret": "...",
  "refresh_token": "...",
  "scope": "..."
}
```

- Access tokens are never persisted.
- `schema_version` must be `1`.

## 5) Persistence rules

- **Load order**: local → blob → bootstrap.
- **Write order**: local → blob.
- **Blob failure behavior**:
  - Missing/invalid blob credentials → startup hard‑fail.
  - Blob unreachable with valid local state → degraded (continue);
    degraded means `gohome_oauth_remote_persist_ok{provider}=0`.
  - Blob unreachable with no valid local state → startup hard‑fail.
- **Bootstrap precedence**:
  - client_id/client_secret always from Nix
  - refresh_token from mutable state
  - on successful refresh, write client_id/client_secret into state

Blob storage:
- Use Hetzner Object Storage (S3‑compatible) as the default target.
- Provision with OpenTofu (bucket + credentials); secrets stored via agenix.

Bootstrap source:
- client_id/client_secret come from the Nix/agenix secret for the provider
  (wired per host in `nix/hosts/*.nix`).

## 6) Provider declaration contract (minimal)

Each plugin must declare:
- `provider` (stable identifier)
- `flow` (`auth_code` or `device`)
- `authorizeURL` (required for `auth_code`)
- `tokenURL` (required for all flows)
- `deviceAuthURL` + `deviceTokenURL` (required for `device`)
- `scope`
- `statePath`

No defaults are inferred by the core library.

Compile‑time enforcement:
- Plugin must implement `OAuthDeclaration()` (interface presence only).

State path constraints:
- `statePath` must be absolute, owned by the gohome user, and created with 0600
  permissions (enforced at startup).

Combined pseudo‑plugin example (auth‑code + metrics):
```go
package evilcorp

import (
    "google.golang.org/grpc"
    "github.com/prometheus/client_golang/prometheus"

    "github.com/joshp123/gohome/core"
    "github.com/joshp123/gohome/internal/oauth"
    evilcorpv1 "github.com/joshp123/gohome/proto/gen/plugins/evilcorp/v1"
)

type Plugin struct{}

var (
    temperatureCelsius = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "gohome_evilcorp_temperature_celsius",
            Help: "Current temperature from EvilCorp device",
        },
        []string{"device_name"},
    )
    humidityPercent = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "gohome_evilcorp_humidity_percent",
            Help: "Current humidity from EvilCorp device",
        },
        []string{"device_name"},
    )
)

func (p *Plugin) Manifest() core.Manifest {
    return core.Manifest{
        PluginID:    "evilcorp_thermo",
        DisplayName: "EvilCorp Thermo",
        Services:    []string{"gohome.plugins.evilcorp.v1.EvilCorpService"},
    }
}

func (p *Plugin) OAuthDeclaration() oauth.Declaration {
    return oauth.Declaration{
        Provider:     "evilcorp_thermo",
        Flow:         "auth_code",
        AuthorizeURL: "https://idp.evilcorp.example.com/oauth2/authorize",
        TokenURL:     "https://idp.evilcorp.example.com/oauth2/token",
        Scope:        "thermo.read thermo.write",
        StatePath:    "/var/lib/gohome/evilcorp-credentials.json",
    }
}

func (p *Plugin) RegisterGRPC(server *grpc.Server) {
    evilcorpv1.RegisterEvilCorpServiceServer(server, &service{})
}

func (p *Plugin) Collectors() []prometheus.Collector {
    return []prometheus.Collector{
        temperatureCelsius,
        humidityPercent,
    }
}

func (p *Plugin) Dashboards() []core.Dashboard {
    return []core.Dashboard{{Name: "evilcorp-overview", JSON: dashboardJSON}}
}

// Example update path in the client/scraper:
// temperatureCelsius.WithLabelValues(deviceName).Set(tempC)
// humidityPercent.WithLabelValues(deviceName).Set(humidityPct)
```

## 7) Reauth + runbook

The core OAuth runner is the only auth entry point:
- Auth‑code: `gohome oauth auth-code --provider <id> --redirect-url <url>`
- Device: `gohome oauth device --provider <id>`

The runner writes the declared `statePath` and mirrors to blob.

Scope changes:
- Deterministic check: if state scope differs from declaration scope, log + emit
  `gohome_oauth_scope_mismatch_total{provider}` and require reauth via the runner.

Refresh failure:
- Single attempt; on failure set `token_valid=0` and return error.

## 8) Metrics (minimal)

- `gohome_oauth_refresh_success_total{provider}`
- `gohome_oauth_refresh_failure_total{provider}`
- `gohome_oauth_token_valid{provider}`
- `gohome_oauth_remote_persist_ok{provider}` (1=last blob write succeeded,
  0=any blob write failure since last success)
- `gohome_oauth_scope_mismatch_total{provider}`

## 9) Non‑goals (explicitly out of scope)

- Multi‑instance support
- Blob locking / TTL / unlock commands
- Retry/backoff policy
- Optional remote store switches
- AGENTS CI enforcement rules
- Rich validation beyond interface presence + schema correctness
