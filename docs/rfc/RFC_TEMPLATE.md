# RFC Template (Lawbot Hub)

- Date:
- Status: Draft | Proposed | Accepted | Rejected
- Audience:

## 1) Narrative: what we are building and why

Explain the user problem and the system intent in plain language.
Anchor explicitly to the **overall project goal** (Lawbot‑Hub North Star) so
future agents do not drift.
Tie explicitly to user goals and workflows.

## 1.1) Non‑negotiables

List the rules that cannot be violated (API‑first, no JSON, ZFC, etc.).

## 2) Goals / Non‑goals

Goals:
- …

Non‑goals:
- …

## 3) System overview

Describe the core components and how they fit together.

## 4) Components and responsibilities

- Component A: …
- Component B: …

## 5) Inputs / workflow profiles

Define the minimum inputs required to run without extra steering.
Include validation rules.

## 6) Artifacts / outputs

List artifacts, formats, and constraints.

## 7) State machine (if applicable)

States, transitions, and rewind rules.

## 8) API surface (protobuf)

List protos and RPCs.

## 9) Interaction model

Explain how users/agents drive the system (API/CLI/UI).

## 10) System interaction diagram

Provide a sequence diagram or flow diagram.

## 11) API call flow (detailed)

Show step‑by‑step requests, responses, and gating rules.

## 12) Determinism and validation

Define deterministic rules and validation criteria.

## 13) Outputs and materialization

Primary outputs + later formats (e.g., Markdown → PDF).

## 14) Testing philosophy and trust

Testing pyramid, trust gates, blast radius.

## 15) Incremental delivery plan

Slices that each produce user‑visible value.

## 16) Implementation order

Ordered steps for execution.

## 16.1) Dependencies / sequencing

- Dependencies: …
- Blocks: …
- Next: …

## 17) Brutal self‑review (required)

Review the RFC as if you were:
- a junior engineer
- a mid‑level engineer
- a senior/principal engineer
- a PM
- an EM
- an external stakeholder
- the end user

Note: This is a personal project, not a real cross‑functional team, but we still
simulate these perspectives to ensure clarity for agents and future readers.

Call out what is unclear, missing, or over‑specified for each persona.
