# RFC: UI Design System for Lawbot‑Hub

- Date: 2025-12-31
- Status: Draft
- Audience: Backend + frontend engineers, platform, orchestrator, agents

## 1) Narrative: what we are building and why

Lawbot‑Hub needs a consistent, trustworthy UI surface for evidence review and legal workflows. Today, each UI app (Vault, Orchestrator, LogicGraph) has its own visual language and interaction patterns. This makes it harder for users to navigate evidence consistently and increases the risk of misinterpretation when outputs are used in formal legal contexts. A **shared design system** provides deterministic, repeatable UI building blocks and shared semantics for evidence, provenance, and workflow states.

The design system is not about visual polish for its own sake; it is about reducing ambiguity, improving trust, and ensuring that every evidence artifact looks and behaves the same across the system. This reduces user error and supports LLM‑driven tools that depend on consistent UI affordances and predictable output structures.

This RFC depends on the **UI + Proto Unification RFC** (`docs/rfc/2025-12-30-ui-proto-unification.md`). Once unification lands, we will author a follow‑on RFC specifically for migrating existing UI components to the design system.

## 1.1) Non‑negotiables

- **ZFC‑compliant**: no heuristic UI logic; UI must reflect data and user intent deterministically.
- **Proto‑first**: component data contracts mirror canonical proto models.
- **Single source of truth** for tokens, components, and patterns.
- **Accessibility baseline** is required (contrast, focus states, keyboard navigation).
- **Minimal blast radius**: incremental adoption; no forced big‑bang redesign.

## 2) Goals / Non‑goals

Goals:
- Shared tokens (color, typography, spacing, radii, elevation).
- Shared evidence primitives (cards, timelines, provenance badges, doc/email/event viewers).
- Shared layout primitives (app shell, nav, filters, detail panes).
- Clear interaction patterns for evidence review and workflow transitions.
- Governance: contribution rules, ownership, and versioning for UI changes.

Non‑goals:
- A full UI redesign.
- A pixel‑perfect brand refresh.
- Rewriting application logic or backend behavior.

## 3) System overview

**Target state:**
- A design system package (`libs/ui`) provides tokens + components + patterns.
- UI shells consume design system components instead of re‑implementing.
- Shared semantics for evidence types (doc/email/event) and provenance badges.

## 4) Components and responsibilities

- **Design Tokens** (`libs/ui/tokens`): color, typography, spacing, radii, elevation, z‑layers.
- **Core Components** (`libs/ui/components`): buttons, inputs, tables, tabs, dialogs, badges.
- **Evidence Components** (`libs/ui/evidence`): doc viewer, email thread, event timeline, provenance chips.
- **Layout Components** (`libs/ui/layout`): app shell, sidebar, filter panel, detail view.
- **Governance**: platform team owns the library; product teams propose changes via PRs.

## 5) Inputs / workflow profiles

Minimum inputs to use the design system:
- Token definitions (CSS variables or TS exports).
- Component API contracts (props) aligned to proto fields.
- A theme entry point consumed by each UI app.

Validation rules:
- UI code must import tokens/components from `libs/ui`.
- New components must document intended data contracts and evidence semantics.

## 6) Artifacts / outputs

- Token definitions (CSS variables + TS helpers).
- Shared UI component library.
- Usage docs/examples (lightweight, in‑repo).

## 7) State machine (if applicable)

Not applicable. This RFC defines structural UI alignment.

## 8) API surface (protobuf)

UI components map to canonical proto types; no UI‑only models for evidence. Examples:
- Doc viewers render `vault.v3.Doc` (or equivalent).
- Email threads render `vault.v3.Thread`/`Message`.
- Event timelines render `vault.v3.Event` with provenance.

## 9) Interaction model

- Engineers build UI shells from shared components.
- Users see consistent filters, evidence cards, and provenance indicators across apps.
- Evidence flows (docs → events → emails) share the same interaction patterns.

## 10) System interaction diagram

```
User -> UI shell (Vault/Orchestrator/Lawbot)
UI shell -> Design system components
Design system -> Shared tokens + evidence primitives
```

## 11) API call flow (detailed)

- UI calls services via shared proto client.
- Responses map directly into design system components.
- No UI layer transforms data into ad‑hoc schemas.

## 12) Determinism and validation

- Design system components must render based on explicit data only.
- Provenance badges and evidence state must be exact reflections of API fields.
- Visual semantics must not imply certainty when provenance is weak.

## 13) Outputs and materialization

Primary outputs:
- Consistent UI patterns and evidence rendering.

Secondary outputs:
- Improved trust in exported evidence packs and timelines (by reducing UI ambiguity).

## 14) Testing philosophy and trust

- UI snapshot tests for shared components.
- Accessibility checks for core components.
- Smoke tests in each app to ensure tokens/components load correctly.

## 15) Incremental delivery plan

1) Define token set and publish in `libs/ui/tokens`.
2) Extract and normalize core components from Vault UI.
3) Port evidence primitives (doc/email/event) into shared library.
4) Migrate Orchestrator UI to shared components.
5) Migrate LogicGraph UI and future Lawbot UI.

## 16) Implementation order

1) Establish tokens + theme entry point.
2) Build core component kit (button/input/table/tabs/dialog).
3) Build evidence primitives with proto‑aligned props.
4) Replace local components in Vault UI first.
5) Roll out to other UIs.

## 17) Brutal self‑review (required)

Junior engineer:
- Resolved: explicit file locations and component categories defined.

Mid‑level engineer:
- Resolved: phased migration plan and component ownership are clear.

Senior/principal engineer:
- Resolved: deterministic UI rules and proto‑aligned contracts avoid semantic drift.

PM:
- Resolved: improves user trust and reduces ambiguity in evidence review.

EM:
- Resolved: clear ownership + incremental migration reduces risk.

External stakeholder:
- Resolved: consistent evidence presentation lowers legal ambiguity risk.

End user:
- Resolved: consistent UI across Vault/Lawbot/Orchestrator reduces confusion.
