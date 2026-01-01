# RFC: OAuth Credentials and Token Rotation

- Date: 2026-01-01
- Status: Draft
- Audience: engineers, operators, plugin authors, agents
- Owner: GoHome core

## 0) Philosophy (applies to this RFC)

- **Nix‑native, declarative**: configuration is code; no runtime edits.
- **If it compiles, it works**: correctness is enforced at build time when possible.
- **Plugins own domain logic; core is mechanical**: auth is centralized, plugins declare
  provider declarations and the OAuth library is agnostic to providers.
- **One obvious way**: a single canonical JSON schema and a single deterministic storage flow.
- **ZFC**: no inference, no heuristics; explicit declarations only.
- **Mutable runtime state is intentional**: the state file is the source of truth
  for refresh tokens after bootstrap; Nix seeds but does not overwrite it.

## 1) Narrative: what we are building and why

GoHome integrates with cloud APIs (Daikin, Tado, etc.) that use OAuth. We need a
**standard, Go‑native OAuth layer** so plugins stop re‑implementing token parsing,
refresh rotation, and persistence. The current ad‑hoc approach caused repeated
breakages (JSON parsing, silent token rotation failures, reauth confusion).

This RFC establishes a shared OAuth module and a state‑file contract that makes
“works first time” the default, mirroring Home Assistant’s OAuth discipline
(centralized auth handling + standardized storage).

## 1.1) Non‑negotiables

- Nix‑native configuration (no runtime config editing)
- Secrets managed via agenix for bootstrap; runtime state is derived and stored
  in the state file + blob
- ZFC compliance: AI decides; GoHome executes mechanically
- Plugins own device logic, but auth handling is standardized
- Access tokens are never written to disk
- After initial authorization, all refresh/rotation must be fully headless

## 2) Goals / Non‑goals

Goals:
- One shared Go library for OAuth parsing + refresh rotation + persistence
- Provider declarations are explicit and plugin‑declared (no inferred defaults)
- Clear reauth strategy and operator runbook
- Metrics + error taxonomy for auth failures
- **Batteries‑included remote persistence (default)**: replicate refresh tokens to
  S3‑compatible blob storage and restore on rebuild when local state is missing

Non‑goals:
- Central OAuth broker service
- GUI for OAuth in GoHome core
- Browser automation for initial auth
- Storing access tokens outside local state files
- Core‑supplied provider defaults or inferred endpoints

V1 scope (explicit):
- Core OAuth runner CLI (auth‑code + device flows)
- Local state + blob persistence with deterministic load/write
- Single‑instance only (no multi‑instance support)

Out of scope (V1):
- Multi‑instance orchestration
- Automatic schema migrations
- Browser automation for auth

## 3) System overview (and HA comparison)

**HA baseline**: Home Assistant uses `config_entry_oauth2_flow` +
`application_credentials` so integrations **do not** implement OAuth. Tokens
are refreshed and stored in HA core. Reauth is standardized.

**GoHome target**: Provide the same “idiot‑proof” behavior by centralizing OAuth
handling in a shared Go package plus a strict state‑file contract. Plugins call
into the shared package; they never parse token files themselves.

### 3.1) HA implementation mapping (reference behavior)

From the HA Daikin Onecta integration:
- **application_credentials.py**: defines authorize/token URLs (no per‑plugin storage).
- **config_flow.py**: uses `AbstractOAuth2FlowHandler`, includes scope in `extra_authorize_data`,
  and supports a reauth flow.
- **daikin_api.py**: uses `OAuth2Session.async_ensure_token_valid()`; raises
  `ConfigEntryAuthFailed` on refresh errors.
- **diagnostics.py**: exposes `oauth2_token_valid` so operators can see token health.

GoHome should mirror this by centralizing OAuth and exposing an explicit token‑health metric.

### 3.2) HA core OAuth details (what actually makes it “idiot‑proof”)

From `homeassistant/helpers/config_entry_oauth2_flow.py` and
`components/application_credentials`:

- **Centralized flow**: `AbstractOAuth2FlowHandler` owns authorize → token exchange →
  config entry creation. Integrations only provide endpoints + scope.
- **Token storage**: token is stored in the config entry (`data["token"]`), not in
  integration code. Refresh updates the config entry atomically via
  `async_update_entry`.
- **Refresh discipline**: `OAuth2Session.async_ensure_token_valid()` uses a lock,
  checks `expires_at` with a 20‑second skew, refreshes, and writes the new token.
- **Error handling**: token requests log provider `error` + `error_description`
  and raise structured errors.
- **PKCE support**: built into `LocalOAuth2ImplementationWithPkce`.
- **Application Credentials**: stored centrally via a `Store` (versioned), validated,
  and exposed via websockets. Deletion is blocked if in use by a config entry.

These behaviors should be mirrored in GoHome’s shared OAuth module.

## 4) Components and responsibilities

- **Core OAuth runner (Go CLI)**: executes auth‑code/device flows and writes
  the initial state file.
- **Provider declarations**: endpoints, scopes, flows (declared by plugins).
- **Token store**: atomic read/write of refresh tokens (0600).
- **Remote token store**: S3‑compatible blob replication for rebuild recovery.
- **Token refresher**: refresh + persist rotation.
- **Plugins**: request access token from shared library; no custom parsing.

Runner discovery:
- The runner is built with the same Nix composition as the core binary (same
  plugin set). It loads the compiled plugin registry and calls
  `OAuthDeclaration()` on each plugin to obtain endpoints and statePath.

## 5) Inputs / workflow declarations

Inputs:
- Bootstrap secret (agenix): client_id, client_secret, optional refresh_token
- State file path: plugin default, overridable via Nix
- Remote token store config (S3‑compatible endpoint/bucket/prefix) for recovery
- Provider declaration: endpoints, scope, flow, statePath

Validation:
- Missing client_id/client_secret/refresh_token → hard error
- Strict JSON schema (snake_case)
- State file permissions must be 0600
- Deterministic load order: local → blob → bootstrap secret
- Schema version must be present and supported
- Remote store credentials are required by default; missing/invalid creds are a
  startup hard‑fail. Explicit opt‑out requires `allowNoRemoteStore = true`.
- Scope rules: if state scope exists and differs from declaration scope → startup
  fail + reauth required; if state scope missing → use declaration scope.

## 5.1) Remote persistence and recovery (batteries‑included)

Default behavior: **write locally, then replicate to blob**. On
startup, if the local state file is missing or invalid, **pull from blob and
rehydrate locally**. This makes rebuilds and disk loss recoverable without reauth.

If remote storage is explicitly disabled (`allowNoRemoteStore = true`), only
local state is used and rebuilds require reauth.

Storage contract:
- Blob object holds only the refresh token state (never access tokens).
- Writes are atomic locally; remote writes are attempted immediately after local
  success.
- Prefer bucket versioning (or object history) for rollback.
- Object retention/lifecycle is an operator policy; V1 does not enforce it.
- Bootstrap secrets are **seed only** and must never overwrite newer local/blob
  state unless the operator explicitly reauths.
- If remote store is enabled but credentials are missing/invalid, startup fails.
- If remote store is enabled, the daemon acquires a blob lock at startup and
  refreshes TTL while running; the runner acquires the same lock only during
  bootstrap/auth and releases on exit.
- Lock TTL is 10 minutes; if the lock exists and is fresh, startup fails
  (multi‑instance guardrail). Stale locks can be cleared with
  `gohome oauth unlock --provider <id>`.

Concurrency:
- Refresh tokens are single‑use/rotating. Multiple writers will invalidate each
  other. **Multi‑instance deployments are unsupported.**

## 5.2) State file schema (local + blob)

State files are the single source of truth for refresh tokens. They **must not**
contain access tokens.

Canonical JSON (snake_case, used by all providers):
```
{
  "schema_version": 1,
  "client_id": "...",
  "client_secret": "...",
  "refresh_token": "...",
  "scope": "home.user"
}
```

One file per provider. No nested provider keys.

Required fields per provider:
- `schema_version`
- `client_id`
- `client_secret`
- `refresh_token`
- `scope` (optional; if missing, use the declaration scope)

Schema versioning:
- `schema_version` must be `1`.
- Unknown versions are rejected at startup; migrations are explicit.
- Any future schema bump must ship with an explicit migration tool/command in the
  same release; no automatic in‑place migration.

Client secret precedence:
- `client_id` + `client_secret` are always taken from the bootstrap secret (Nix).
- `refresh_token` is taken from the mutable state file.
- On write, the state file is updated with the current `client_id`/`client_secret`
  from Nix so rotations are persisted.
- If no refresh occurs, the state file may retain old secrets; runtime always uses
  the bootstrap secrets regardless of state file content.
- If refresh fails with `invalid_client`, reauth is required.

Format:
- JSON only.
- Keys must be snake_case.

Default paths (per Nix module):
- Daikin: `/var/lib/gohome/daikin-credentials.json`
- Tado: `/var/lib/gohome/tado-token.json`

Nix override:
- `services.gohome.oauth.statePath.<provider>` overrides the plugin default.

## 5.3) Remote blob keying + recovery policy

Key format (default): `gohome/oauth/<provider>.json`

Remote config (S3‑compatible):
- endpoint
- bucket
- prefix
- region (optional for non‑AWS)
- access_key_id / secret_access_key (agenix)
- session token (optional)
- least‑privilege policy scoped to `prefix` only (avoid bucket‑wide access)

Recovery behavior:
- If local state exists and parses → use it.
- If local state is missing/invalid → fetch blob and rehydrate locally.
- If blob is missing/invalid → use bootstrap secret if present; otherwise fail
  with actionable error; reauth required.
- Bootstrap secrets are only used when both local and blob are missing/invalid.
- If blob is configured but unreachable and local exists → start in degraded mode.
- If blob is configured but unreachable and local is missing/invalid → startup fails.

Failure classification:
- **Credential errors** (missing/invalid blob keys) → startup hard‑fail.
- **Blob unreachable** (network/timeout/5xx) → degraded if local valid, else hard‑fail.

Write behavior:
- After a successful refresh, write local state atomically, then write blob.
- Never overwrite local state from blob during steady‑state runtime.
- Blob write failures must surface via metrics and logs; local remains source of truth.
  The refresh is considered successful for runtime use, but the system is degraded
  until blob persistence recovers.
- Degraded means `token_valid=1` but `gohome_oauth_remote_persist_ok{provider}=0`.

Conflict handling:
- If blob write fails due to ETag mismatch → log + metric, do not overwrite.
- Operator must resolve by selecting a single writer or forcing a reauth.

## 5.4) Nix configuration sketch (expected shape)

Example (names may change during implementation):
```
services.gohome.oauth = {
  remoteStore = {
    enable = true;
    endpoint = "https://s3.hetzner.com";
    bucket = "gohome-secrets";
    prefix = "gohome/oauth";
    region = "eu-central";
    accessKeyFile = config.age.secrets.oauth-s3-access-key.path;
    secretKeyFile = config.age.secrets.oauth-s3-secret-key.path;
  };
  # Optional explicit opt‑out (risk accepted)
  allowNoRemoteStore = false;
  # Optional per‑provider state path overrides
  # statePath = { daikin_onecta = "/var/lib/gohome/daikin-credentials.json"; };
};
```

## 5.5) Plugin declaration (minimal, explicit, mechanical)

Plugins declare a single OAuth declaration (no ad‑hoc parsing). Example shape:
```
oauthDeclaration = {
  provider = "<provider_id>";
  flow = "auth_code"; // or "device"
  authorizeURL = "<authorize_url>";
  tokenURL = "<token_url>";
  scope = "<scope>";
  statePath = "<state_json_path>";
};
```
The shared OAuth module owns parsing, refresh, rotation, persistence, and metrics.

## 6) Provider declaration contract (normative)

Each plugin must declare:
- `provider` (stable identifier used for metrics + blob key)
- `flow` (`auth_code` or `device`)
- `authorizeURL` (required for `auth_code`)
- `tokenURL` (required for all flows)
- `deviceAuthURL` + `deviceTokenURL` (required for `device`)
- `scope`
- `statePath` (JSON file path)

No defaults are inferred by the core library. If a field is missing, validation
fails at startup.

Compile‑time enforcement:
- Plugins must provide a concrete Go declaration struct (e.g. `oauth.Declaration`)
  via a typed interface on the plugin (e.g. `OAuthDeclaration() oauth.Declaration`).
- Build time validates interface presence only; field constraints are validated
  at startup.

Declaration constraints (enforced):
- `provider` is a stable slug (lowercase, digits, underscore).
- URLs must be absolute `https://` URLs.
- `statePath` must be absolute.
- `flow` is one of `auth_code` or `device`.
- `authorizeURL` is required for `auth_code`.
- `deviceAuthURL` + `deviceTokenURL` are required for `device`.
- `scope` is required in the declaration (use empty string only if the provider
  explicitly requires no scope).

Documentation enforcement:
- Provider endpoints (including device flow endpoints) must be documented in the
  plugin’s `AGENTS.md` alongside the declaration source of truth.
- Required AGENTS block (CI enforced): `OAuth Endpoints:` with `authorizeURL`,
  `tokenURL`, and (if device flow) `deviceAuthURL`, `deviceTokenURL`.

State path override:
- `statePath` in the declaration is the default; Nix can override per host via
  `services.gohome.oauth.statePath.<provider>`.

## 7) Reauth strategy

- On `invalid_grant`:
  - Fail fast with explicit operator message + metric.
- If provider uses device activation:
  - Restart device activation flow and replace refresh token on success.
- On `invalid_client` or `invalid_scope`: fail fast and instruct operator to
  re‑register or fix credentials.

## 8) Runbook (how to reauth)

This is the **bootstrap + break‑glass** path only. Normal operation is fully headless.

1) Run the core OAuth runner (required in V1):
   - Auth‑code flow: `gohome oauth auth-code --provider <id> --redirect-url <url>`
   - Device flow: `gohome oauth device --provider <id>`
2) Follow the human UI steps documented in the plugin’s `AGENTS.md`.
3) The runner exchanges tokens and writes the state file declared by the plugin.
   - Output path is the declared `statePath`.
   - File must contain `schema_version: 1` and required fields.
   - Runner exits 0 only after a successful write and prints the path written.
   - If remote store is enabled, the runner writes local then blob.
4) Start GoHome and confirm `gohome_oauth_token_valid{provider}=1` and
   the plugin scrape metric is healthy.

If startup fails with a scope mismatch (state vs declaration), rerun the OAuth
runner to reauth with the new scope.

Rebuild recovery (no reauth):
1) Ensure blob credentials are available (agenix).
2) Start GoHome; it will rehydrate local state from blob if missing.
3) Confirm `gohome_oauth_token_valid{provider}=1` and scrape metrics are healthy.

Rebuild without blob storage (`allowNoRemoteStore = true`):
1) Reauth is required (bootstrap secret may be stale).

Device activation is handled entirely by the core runner using the plugin’s
device flow endpoints; the only manual step is entering the user code in the
vendor UI (documented in `AGENTS.md`).

## 9) Metrics & alerts

Expose the following per provider:
- `gohome_oauth_refresh_success_total{provider}`
- `gohome_oauth_refresh_failure_total{provider}`
- `gohome_oauth_invalid_grant_total{provider}`
- `gohome_oauth_last_success_timestamp{provider}`
- `gohome_oauth_token_valid{provider}` (mirrors HA diagnostics `oauth2_token_valid`)
- `gohome_oauth_remote_persist_ok{provider}` (1=last blob write succeeded, 0=any
  blob write failure since the last success; no time decay)
- `gohome_oauth_blob_write_success_total{provider}`
- `gohome_oauth_blob_write_failure_total{provider}`
- `gohome_oauth_blob_read_success_total{provider}`
- `gohome_oauth_blob_read_failure_total{provider}`
- `gohome_oauth_rehydrate_success_total{provider}`
- `gohome_oauth_rehydrate_failure_total{provider}`
- `gohome_oauth_blob_write_stale_total{provider}` (ETag mismatch)

## 10) Threat model + file guarantees

- Refresh tokens must be stored on disk to support long‑running services.
- State files must be 0600 and owned by the gohome user. Nix modules must set
  ownership on bootstrap copy (install ‑m 0600 ‑o gohome ‑g gohome).
- Access tokens are never persisted.
- Writes must be atomic: write temp → `fsync` → rename.
- Remote blob credentials are secrets and must be managed via agenix.
- Remote blobs must never contain access tokens; only refresh token state.
- Blob storage must use TLS in transit and server‑side encryption at rest.
- State files contain `client_secret`; treat them as secrets.
- Never log tokens or client secrets; redact on error.
- Least‑privilege blob credentials should only permit read/write under the
  configured prefix to avoid cross‑provider leakage.

## 11) Error taxonomy (operator actions)

- `invalid_grant`: token revoked or rotated; reauth required.
- `invalid_client`: wrong client_id/secret; fix credentials.
- `invalid_scope`: mismatched scope; reauth with correct scope.
- `scope_mismatch`: declaration vs state; reauth required.
- `rate_limit`: backoff + operator notice.
- `blob_missing`: no local state + blob missing; reauth required.
- `blob_conflict`: multiple writers; enforce single writer or reauth.
- `blob_write_failed`: local OK, remote failed; monitor and fix blob access.

## 12) Success criteria + user impact

Success criteria:
- Plugins never parse credential files directly.
- Refresh rotation succeeds for Daikin + Tado without manual intervention.
- When blob storage is configured, rebuild restores a valid refresh token without reauth.
- Metrics show a clean path to diagnose auth failures.

User impact:
- One‑time auth only; steady‑state is headless.
- Rebuilds do not force reauth if blob storage is configured.
- Operators get explicit, actionable errors instead of silent failures.

## 13) API surface (protobuf)

No new RPCs required.

## 14) Determinism and validation

- Strict parsing with explicit schema and snake_case keys
- Fail fast on missing credentials
- Atomic state file writes
- Deterministic load order (local → blob → bootstrap)
- Provider quirks must be explicit in declarations, not inferred at runtime.
- Retry policy is fixed and mechanical:
  - Deterministic policy; no adaptive backoff or inference.
  - Refresh runs off‑path; request/scrape paths never sleep.
  - If no valid access token is cached, requests fail fast until refresh succeeds.
  - 1 refresh attempt per cycle.
  - 1 retry after a fixed delay (30s) only for 5xx/429/network errors.
  - No retries for `invalid_grant`, `invalid_client`, `invalid_scope`.
- In‑process refresh is single‑flight (one refresh in flight at a time).
- Validation phases:
  - Compile‑time: interface presence only (plugin implements `OAuthDeclaration()`).
  - Startup: declaration field constraints, config, state file schema, and
    permissions validated before plugins start.
  - Runtime: periodic token validation/refresh to confirm tokens remain valid.
- Token validity semantics:
  - Access tokens are cached in memory only.
  - Refresh when `expires_at <= now + 30s` (fixed skew).
  - `token_valid` is true when last refresh succeeded and access token is not expired.

## 15) Testing philosophy and trust

- Unit tests for JSON parsing
- Mock OAuth server tests for refresh rotation
- Remote store tests (blob read/write, conflict, missing)
- Regression tests for key casing errors

## 16) Incremental delivery plan

V1 (reliability + shared library + blob recovery):
1) Introduce `internal/oauth` package (parsing + refresh + persistence)
2) Build core OAuth runner CLI (auth‑code + device flows)
3) Add remote token store (S3‑compatible) + recovery flow
4) Migrate Tado + Daikin to shared package
5) Add metrics + runbook (AGENTS docs)

## 17) Implementation order

1) Library + tests
2) Core OAuth runner CLI
3) Remote token store + recovery
4) Plugin migrations
5) Documentation + runbooks

## 18) Brutal self‑review (required)

- Junior: Example state file schema + recovery runbook included.
- Mid: Atomic write strategy + conflict behavior + load order specified.
- Senior: Threat model expanded (TLS, encryption, secrets handling).
- PM: Success criteria + user impact included.
- EM: Rollout captured in delivery plan; ownership is GoHome core.
- External stakeholder: Jargon reduced, actionable errors listed.
- End user: Explicit auth‑failure actions + rebuild recovery steps included.
- Reviewer gaps addressed:
  - Password grant removed from V1 behavior; deprecated and not required.
  - Single canonical flat JSON schema with schema_version.
  - Provider endpoints live in plugin code/AGENTS; RFC contains no defaults.
  - Core OAuth runner is required in V1 and is the single auth trigger.
  - Remote store is V1 default with explicit load/write and degraded‑mode policy.
  - Multi‑instance deployments are explicitly unsupported.
  - Schema migrations are explicit; unknown versions fail startup.
  - Runner discovery uses compiled plugin registry + `OAuthDeclaration()`.
  - Validation timing clarified: compile‑time interface, startup field constraints.
  - Remote persist metric semantics defined (last blob write success).
  - AGENTS endpoint block is required and CI‑enforced.
  - Runner output contract and blob lock guardrail specified.
  - State path override via Nix is supported.
  - Client secret precedence for rotation is explicit.
  - Lock TTL + unlock path defined; daemon owns lock during runtime.
  - Scope mismatch behavior is explicit (startup fail + reauth).
  - Refresh off‑path contract is explicit (fail‑fast, single‑flight).
  - Remote store opt‑out is explicit (`allowNoRemoteStore = true`).
