# RFC: UI Component Migration to the Lawbot‑Hub Design System

- Date: 2025-12-31
- Status: Draft
- Audience: Frontend engineers, platform, orchestrator, agents

## 1) Narrative: what we are building and why

After we unify proto and UI foundations, we must **migrate existing UI components** to the shared design system so that evidence review is consistent and reliable across Vault, Orchestrator, and Lawbot. Today, Orchestrator embeds Vault UI components directly (see `docs/rfc/2025-12-30-orchestrator-tool-requests.md`), which creates coupling, inconsistent styling, and brittle ownership. This migration replaces ad‑hoc embedding with shared, versioned components that are explicitly designed for reuse.

The goal is to reduce UI drift and make evidence presentation deterministic. This is a correctness requirement, not cosmetic: evidence artifacts shown in Orchestrator must be identical in meaning and affordance to those in Vault, otherwise we introduce legal risk.

## 1.1) Non‑negotiables

- Must build on the **UI + Proto Unification RFC** (`docs/rfc/2025-12-30-ui-proto-unification.md`).
- Must build on the **UI Design System RFC** (`docs/rfc/2025-12-31-ui-design-system.md`).
- ZFC‑compliant: no heuristic UI behavior or hidden data transformations.
- No breaking of core evidence workflows during migration.
- Migration is staged and reversible; no large‑bang rewrite.

## 2) Goals / Non‑goals

Goals:
- Replace embedded Vault UI inside Orchestrator with **design‑system evidence primitives**.
- Migrate Vault UI to shared components without feature regression.
- Migrate Orchestrator UI to shared components while preserving tool‑request flow.
- Produce consistent evidence rendering (doc/email/event/provenance) across apps.

Non‑goals:
- New visual style or brand redesign.
- Overhauling application logic or backend behavior.
- Changing the evidence model (handled in proto RFCs).

## 3) System overview

- **Shared UI components** in `libs/ui` become the only supported building blocks.
- **UI shells** (Vault/Orchestrator/Lawbot) import shared components and provide routing/state only.
- Orchestrator no longer embeds Vault UI; it renders **shared evidence views** directly.

## 4) Components and responsibilities

Shared evidence components to migrate first (parity targets):
- Doc list + doc detail (current Vault docs desk)
- Thread list + thread detail (Vault threads desk)
- Event list + event detail (Vault events desk)
- Provenance badges and evidence chips

Owners:
- **Platform**: design system library + shared evidence primitives.
- **Vault UI**: reference implementation + parity sign‑off.
- **Orchestrator UI**: tool‑flow integration + deep links to Vault.

## 5) Inputs / workflow profiles

Minimum inputs:
- Stable shared components and tokens from the design system RFC.
- Shared proto client for evidence data.
- Updated UI shells that can import shared components.

Validation rules:
- Evidence views must be visually and behaviorally consistent across apps.
- No UI‑local data transforms for evidence (only presentation).

## 6) Artifacts / outputs

- Updated Vault UI using shared components.
- Updated Orchestrator UI using shared components.
- Deprecation of embedded Vault UI inside Orchestrator.

## 7) State machine (if applicable)

Not applicable. This is a migration plan for UI components.

## 8) API surface (protobuf)

No new API contracts. Uses existing proto surface established by the unification RFC.

## 9) Interaction model

- Users interact with evidence through the same UI primitives in Vault and Orchestrator.
- Orchestrator can open evidence in Vault (deep‑link) but does not reuse Vault UI code directly.

## 10) System interaction diagram

```
User -> Orchestrator UI shell
Orchestrator UI -> Shared evidence components (libs/ui)
Shared components -> Shared proto client -> Vault API
```

## 11) API call flow (detailed)

- Orchestrator requests evidence via shared proto client.
- Evidence components render the responses identically to Vault.
- Deep links to Vault remain for full context.

## 12) Determinism and validation

- Evidence cards/timelines use shared, deterministic formatting.
- Provenance badges use shared rules; no app‑specific variants.
- Rendering output must match Vault’s evidence views for identical data.

## 13) Outputs and materialization

Primary outputs:
- Consistent evidence presentation across all UI shells.

Secondary outputs:
- Reduced coupling between Orchestrator and Vault UI code.

## 14) Testing philosophy and trust

- Screenshot/visual diffs for evidence components across apps.
- Smoke tests for Vault + Orchestrator with shared components enabled.
- Manual QA: verify identical evidence rendering across apps for a sample dataset.

## 15) Incremental delivery plan

1) **Vault UI migration**: replace local components with shared evidence primitives.
2) **Orchestrator migration**: swap embedded Vault UI for shared evidence primitives.
3) **Lawbot UI integration**: adopt shared components for evidence views (if/when Lawbot UI exists).

## 16) Implementation order

1) Confirm design system tokens + components are stable.
2) Replace Vault UI evidence components with shared equivalents.
3) Replace Orchestrator embed with shared evidence components.
4) Remove legacy embed paths and clean up dependencies.
5) Ship a parity report with before/after screenshots for docs/threads/events.

## 17) Success criteria (explicit parity)

- For the same doc/thread/event IDs, Vault and Orchestrator render:
  - identical titles, timestamps, participants, and provenance indicators
  - identical ordering and filtering semantics
  - identical evidence chips and deep links
- No Orchestrator references to Vault UI code remain.
- All evidence views use shared components from `libs/ui`.

## 18) Brutal self‑review (required)

Junior engineer:
- Resolved: component list and parity targets are explicit.

Mid‑level engineer:
- Resolved: success criteria + parity checks defined; rollback is simple (revert shared component import).

Senior/principal engineer:
- Resolved: staged migration with explicit parity report and no‑embed rule.

PM:
- Resolved: user‑visible improvements (consistent evidence views) are explicit.

EM:
- Resolved: ownership and phased plan reduce coordination risk.

External stakeholder:
- Resolved: reduces evidence ambiguity in legal outputs by enforcing consistent rendering.

End user:
- Resolved: consistent evidence rendering across tools; fewer surprises.
