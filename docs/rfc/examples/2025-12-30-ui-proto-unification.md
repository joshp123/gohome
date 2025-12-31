# RFC: Unified UI + Proto Layer Across Lawbot‑Hub

- Date: 2025-12-30
- Status: Draft
- Audience: Backend + frontend engineers, platform, orchestrator, agents

## 1) Narrative: what we are building and why

Lawbot‑Hub’s mission is to deterministically produce defensible legal outputs from primary evidence, with full auditability. That requires **consistent, machine‑readable APIs** and a **coherent UI** across Vault (evidence), Lawbot (research), and Orchestrator (workflow). Today, UI logic and protobuf definitions are copied in multiple places, which causes drift, inconsistent behaviors, and extra maintenance. This RFC defines a single canonical proto source and a shared UI foundation so that all services and UIs stay aligned, reduce duplication, and preserve the integrity of outputs that may end up in formal legal documents.

The user workflow must be stable and auditable: evidence in Vault should look and behave the same when rendered in Orchestrator or Lawbot, and every API consumer should rely on identical contracts. This is especially critical because downstream LLMs will treat these outputs as canonical. Any API drift or UI mismatch is a correctness risk.

This RFC intentionally **enables** the UI Design System RFC (`docs/rfc/2025-12-31-ui-design-system.md`). Once proto + UI unification is in place, we can standardize shared tokens and components. After the design system lands, we will write a follow‑on RFC to migrate existing UI components to that system.

## 1.1) Non‑negotiables

- **Proto‑first** for every service boundary; no JSON‑only contracts.
- **ZFC‑compliant**: no heuristic routing or hidden inference in UI or client code.
- **Single source of truth** for `.proto` definitions in this repo.
- **Minimal blast radius**: changes are staged and verifiable; no large rewrite required to adopt.
- **Go‑first** for backend services; UI uses Vite + React 18 + React Router.
- **Breaking proto changes are allowed** (nothing is live); move fast and fix forward.
- **No submodule edits** without explicit request.

## 2) Goals / Non‑goals

Goals:
- One canonical proto location used by **all** services and UIs.
- One shared UI foundation (components + client + styling) to eliminate UI drift.
- Shared codegen and CI guardrails to prevent proto divergence.
- Fast iteration across components without re‑implementing API or UI primitives.

Non‑goals:
- Full visual redesign of UI.
- Forcing a single runtime deployment for all apps.
- Changing product logic or business rules in this RFC.
- Heavy migration planning; we control the repo and can move quickly.

## 3) System overview

**Current state (observed):**
- Multiple UI apps: `lawbot-vault/ui`, `orchestrator/ui`, `ui/apps/logicgraph`.
- Multiple proto copies: root `proto/`, `lawbot-vault/proto/docs/api/v3`, `lawbot/proto/docs/api/v3`, `casepipe/docs/api/v3`, `orchestrator/ui/public/proto`, `logicgraph/internal/casepipepb`.
- UI proto loading is sometimes direct from `public/proto` copies.

**Target state:**
- **Root `proto/` is the only source of truth** for buildable/prod protos.
- Shared UI foundation (library + API client) consumed by each UI app.
- Backends and UIs consume generated code from the same root protos.
- Vendored upstream snapshots may remain **read‑only** under `lawbot-vault/upstream/**` but are excluded from builds.

## 4) Components and responsibilities

- **Root Proto Registry (`proto/`)**: canonical protobuf definitions for Vault, Lawbot, Orchestrator, LogicGraph.
- **Codegen Scripts (`scripts/`)**: generate Go/TS outputs; enforce regeneration on change.
- **Shared UI Foundation (`libs/ui`)**: shared components, layout primitives, and design tokens.
- **Shared Proto Client (`libs/proto-client`)**: protobufjs loader + transport + typed helpers.
- **App Shells** (`lawbot-vault/ui`, `orchestrator/ui`, `ui/apps/logicgraph`): thin wrappers that compose shared UI and inject app‑specific pages only.

Ownership (default):
- **Protos**: platform (owner of API contracts).
- **Shared UI + proto client**: platform; each product team contributes via PRs.

## 5) Inputs / workflow profiles

Minimum inputs to run:
- Canonical `.proto` files in `proto/` with correct `go_package` and package names.
- Standard codegen invocations (Go + TS) that require no manual edits.
- UI apps reference shared UI + proto client packages.

Validation rules:
- Any proto change must regenerate outputs in the same commit.
- No `.proto` copies outside root `proto/` (enforced by CI check, with allowlist for `lawbot-vault/upstream/**`).
- UI builds must pass using only shared proto client (no `public/proto` copies).

## 6) Artifacts / outputs

- Generated Go packages from root protos (for services).
- Generated TS proto descriptors/types (for shared client).
- Shared UI package and shared proto client library.
- App bundles produced from shared libs.

## 7) State machine (if applicable)

Not applicable. This RFC defines structural alignment rather than runtime state.

## 8) API surface (protobuf)

Canonical (root) proto families:
- `proto/lawbot/orchestrator/v1/*.proto`
- `proto/lawbot/logicgraph/v1/*.proto`
- `proto/lawbot/vault/v3/*.proto` (re‑homed from current `docs/api/v3` copies)

`go_package` policy:
- Must point to generated module roots (e.g., `lawbot-hub/orchestrator/gen/...`).
- Versioned packages stay versioned in path (e.g., `/v3/`).

## 9) Interaction model

- **Agents/engineers** update root protos and run codegen.
- **Backends** compile Go from generated outputs.
- **UIs** import shared UI + proto client to render consistent views.
- **Users** interact with consistent UI workflows across Vault/Lawbot/Orchestrator.

## 10) System interaction diagram

```
User -> UI shell (Vault/Orchestrator/Lawbot)
UI shell -> Shared UI + Shared Proto Client
Shared Proto Client -> Service API (gRPC/HTTP)
Service API -> Root protos (canonical)
```

## 11) API call flow (detailed)

1) Developer edits a proto in `proto/`.
2) Codegen runs (Go + TS). Outputs are updated in the same commit.
3) Shared proto client uses generated descriptors/types to make requests.
4) UI shells call shared client; render shared components.
5) Services implement canonical proto contracts; responses validated by types.

Gating rules:
- CI rejects commits with proto changes but no regenerated outputs.
- CI rejects `.proto` files outside `proto/` (allowlist: `lawbot-vault/upstream/**`).

## 12) Determinism and validation

- No heuristic API behavior: client and UI strictly follow proto‑declared fields.
- No ad‑hoc JSON response shaping.
- Versioned protos: changes tracked explicitly in Git.
- Any UI logic that depends on data meaning must be deterministic and documented.

## 13) Outputs and materialization

Primary outputs:
- Canonical proto contracts.
- Shared UI library and shared proto client.

Secondary outputs:
- Application bundles that are consistent in behavior and data rendering.

## 14) Testing philosophy and trust

- **Proto drift check**: fail if `.proto` files exist outside `proto/` (allowlist upstream snapshot dir).
- **Codegen check**: diff generated output vs repo; fail on mismatch.
- **UI build tests**: build each UI shell against shared libs.
- **Smoke tests**: launch each UI app and verify data loads.
- **Service tests**: `go test` per module (existing workflows).

## 15) Incremental delivery plan

1) **Declare canonical proto root**: inventory all copies and define root as sole source.
2) **Centralize codegen**: update scripts to generate from root for Go + TS.
3) **Shared proto client**: create TS client library and update one UI to use it.
4) **Shared UI library**: extract common components, migrate UI shells.
5) **Finalize**: remove remaining proto copies; lock in CI guardrails.

## 16) Implementation order

1) Freeze/track all proto duplicates; root `proto/` becomes canonical.
2) Re‑home Vault/Casepipe API protos under root (retain package names).
3) Update Go codegen to use root protos:
   - Example: `scripts/gen_casepipe_proto_go.sh` (or successor) generates from `proto/` only.
4) Add TS proto codegen:
   - Output to `libs/proto-client/gen` and consumed by UIs.
5) Migrate one UI (Vault) to shared proto client.
6) Extract shared UI components into `libs/ui` and migrate remaining apps.
7) Remove legacy proto copies and `public/proto` assets.

## 16.1) Operational appendix (commands)

Illustrative commands (exact scripts may change, but intent is stable):
- Go codegen: `scripts/gen_casepipe_proto_go.sh` (updated to point at `proto/` root).
- TS codegen: `scripts/gen_proto_ts.sh` (new; writes to `libs/proto-client/gen`).
- Drift check: `scripts/check_proto_single_source.sh` (new; allowlists `lawbot-vault/upstream/**`).
- UI build smoke: `cd lawbot-vault/ui && npm run build` (repeat for other apps).

## 17) Brutal self‑review (required)

Junior engineer:
- Resolved: codegen commands and artifact locations are specified in 16.1.

Mid‑level engineer:
- Resolved: `go_package` policy and migration order are explicit; UI extraction is phased by app.

Senior/principal engineer:
- Resolved: breaking proto changes explicitly allowed; shared‑UI size handled by keeping app shells thin and shared libs focused on primitives.

PM:
- Resolved: ties directly to consistent evidence workflows and reduced drift in legal outputs.

EM:
- Resolved: ownership for shared UI/proto client is assigned (platform).

External stakeholder:
- Resolved: lowers legal risk by ensuring evidence appears identically across tools.

End user:
- Resolved: consistent UI across docs/emails/events; fewer surprises and re‑learning.
