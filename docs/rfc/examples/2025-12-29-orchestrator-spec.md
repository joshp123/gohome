# RFC: Orchestrator Kernel (Go + Protobuf, ZFC)

- Date: 2025-12-29
- Status: Draft
- Audience: cross‑functional (engineers, PMs, EMs, adjacent teams)

## 1) Narrative: what we are building and why

We are building an internal system that lets a user produce **properly sourced** legal
letters and memos with LLM assistance. The user experience we are anchoring on is:
“give the system my intent and key sources, then let it gather evidence, attach
citations, and generate a draft I can stand behind.”

This system must:
- **Start from intent** (what we want to say), not from evidence collection.
- **Never invent evidence**; everything must map to verifiable sources.
- **Iterate quickly** when new evidence or corrections appear.

To achieve this, we are unifying three systems under a single orchestration kernel:
- **CasePipe** for evidence corpus (emails/docs/events).
- **LawBot** for statutes/caselaw and citations.
- **LogicGraph** for span resolution (exact quotes and offsets).

The orchestrator is the “thin deterministic shell” around LLM reasoning. It allows
free‑text outputs from the LLM, but **all evidence pointers and citations are formal**
and validated. This is the ZFC principle in practice: AI can reason, the kernel
only validates and records.

API‑first design: the **protobuf API is the product**. CLI and UI are thin wrappers
over the same API. We do not expose JSON endpoints.

Canonical user workflows (input anchors):
- Dec 9 forensic reply: docs/user_intent/2025-12-09-email/*
- UWV deskundigenoordeel: docs/user_intent/uwv/deskundigenoordeel_workflow.md

## 1.1) Non‑negotiables (must‑follow rules)

- API‑first: protobuf API is the product; CLI/UI are wrappers
- No JSON endpoints for orchestration (ever)
- ZFC: LLM outputs free text only; kernel owns formal evidence + citations
- Explicit ID conventions (EvidenceRef + Artifact ids)

## 2) High‑level system shape

The orchestrator drives a strict state machine and emits versioned artifacts at
each stage. Evidence retrieval is delegated to CasePipe/LawBot adapters. Span
resolution uses LogicGraph semantics. The UI is a protobuf client that navigates
runs and artifacts (no JSON on the wire).

This design is intentionally **high‑level enough** to understand without codebase
knowledge, but **specific enough** to implement directly.

## 3) Components and responsibilities

- Orchestrator (Go): state machine, artifact store, gating, validation, API
- CasePipe: deterministic evidence export (emails/docs/events)
- LawBot: statutes/caselaw + citations
- LogicGraph: span resolution (SpanLocator → SpanRef)
- pi library: shared LLM adapter (free‑text outputs only)
- UI: protobuf client for run navigation and inspection
- CLI: thin wrapper over OrchestratorService (protobuf only)

## 4) Why this design (tradeoffs)

- **Intent‑first** flow prevents premature evidence fishing and matches how users
  actually write. It also prevents wasted collection when intent changes.
- **Protobuf‑first** keeps contracts rigid and prevents drift between services.
- **ZFC‑aligned** boundaries prevent hallucinated citations and keep the system
  auditable.
- **Iteration via rewinds** lets users incorporate new facts without restarting.

This trades some early complexity (formal artifacts and state machine) for
long‑term trust and repeatability.

Design principle: explicit over implicit. There should be one obvious way to
identify evidence, artifacts, and transitions.

## 5) Inputs (workflow profiles)

Profiles are the minimal inputs required to run without extra steering.

Proto: proto/lawbot/orchestrator/v1/profile.proto
Examples:
- docs/user_intent/2025-12-09-email/profile_example.textproto
- docs/user_intent/uwv/profile_example.textproto

Required fields:
- profile_id
- intent_seed_path (must exist in repo)
- output_type (free text)
- tone_goal (free text)
- required_sections (>=1)
- key_evidence_pins (optional; if provided must be valid EvidenceRef entries)

Validation rules:
- Fail if output_type or tone_goal is empty
- Fail if intent_seed_path does not exist

## 6) Artifacts (what gets produced)

Protos: proto/lawbot/orchestrator/v1/artifacts.proto
All artifacts include AuditMeta (id, version, created_at_iso, doc_refs).

Required artifacts:
- IntentSeed (free text)
- KeyEvidencePins (formal ids)
- MessageSkeleton (free text)
- EvidenceNeeds (free text)
- EvidenceSet (formal ids)
- EvidenceNeedMap (optional MVP)
- SpanLocator / SpanRef (formal quote resolution)
- CitationLedger (statement_id → SpanRef)
- DraftText (final text; references CitationLedger)

Constraints:
- EvidenceRef.source_id must be stable and traceable (CasePipe/LawBot)
- SpanRef must trace to EvidenceRef via doc_uid
- DraftText must reference CitationLedger

Explicit ID conventions (one obvious way):

EvidenceRef.source_id (string formats)
```
CasePipe email:  doc:email:<item_uid>
CasePipe doc/pdf: doc:sha256:<sha256>
LawBot statute:  law:<jurisdiction>:<code>:<section>
LawBot case:     case:<jurisdiction>:<ecli_or_reporter_id>
```

Artifact ids
```
artifact_id = sha256(<profile_id>|<artifact_type>|<version>)
version = monotonically increasing integer per artifact type
```

## 7) State machine (what advances when)

States:
- INTENT_SEED → SKELETON → EVIDENCE_NEEDS → EVIDENCE_BINDING → SPAN_RESOLUTION → DRAFT

Allowed rewinds:
- SKELETON, EVIDENCE_NEEDS, EVIDENCE_BINDING, SPAN_RESOLUTION

Rewind semantics:
- Rewind does NOT delete artifacts
- Rewind creates a new version chain
- Re‑runs reuse artifacts unless explicitly invalidated

## 8) API surface (protobuf)

Protos:
- common.proto, artifacts.proto, profile.proto, state.proto, service.proto

RPCs:
- CreateRun, GetRun, ListArtifacts, GetArtifact
- Advance, Rewind, ResolveSpans, ValidateRun
- UpsertIntentSeed (optional), ListProfiles (optional)

FE↔BE rule: protobuf only, no JSON endpoints for orchestration.

## 8.1) User interaction model (API‑first)

Users (or an LLM in testing) explicitly drive the state machine via API calls.
There is **no auto‑advance**. Each stage requires an explicit `Advance` call so
the user can review artifacts at each step before moving on.

Primary interface: protobuf API (OrchestratorService).  
Secondary interfaces: CLI and UI are thin wrappers over the same API.

User action → system response (binding to stages):
- Provide intent + seed evidence pins (docs to start from) → CreateRun
- Review skeleton and edit if needed → GetArtifact/UpsertIntentSeed → Advance
- Review evidence needs → Advance to binding
- Review bound evidence list and remove/add items → Rewind or Advance
- Review draft + citations → ValidateRun → finalize or rewind

## 9) Narrative workflows (user‑driven)

### A) Dec 9 forensic reply workflow (user perspective)

1) User writes seed outline + pins the Dec 9 letter (seed evidence).
   - Input files: docs/user_intent/2025-12-09-email/seed_points.md + letter_jp_2025-12-09.md
   - API: CreateRun(profile=example-2025-12-09-forensic-reply)
   - State: INTENT_SEED

2) User advances to outline (skeleton) and reviews section order.
   - API: Advance(SKELETON)
   - Artifact: MessageSkeleton
   - If edits needed: UpsertIntentSeed then Advance(SKELETON) again.

3) User advances to evidence needs, checks missing items.
   - API: Advance(EVIDENCE_NEEDS)
   - Artifact: EvidenceNeeds

4) User advances to binding, reviews the bound evidence set.
   - API: Advance(EVIDENCE_BINDING)
   - Artifact: EvidenceSet + EvidenceNeedMap
   - If evidence missing, user rewinds to EvidenceNeeds and adds guidance.

5) User resolves spans and advances to draft.
   - API: ResolveSpans → Advance(SPAN_RESOLUTION) → Advance(DRAFT)
   - Artifacts: SpanRef, CitationLedger, DraftText

6) User validates, then iterates or finalizes.
   - API: ValidateRun
   - If errors: Rewind to EVIDENCE_BINDING or SPAN_RESOLUTION

### B) UWV deskundigenoordeel workflow (user perspective)

1) User loads UWV intent profile and key evidence gaps.
   - Input: docs/user_intent/uwv/deskundigenoordeel_workflow.md
   - API: CreateRun(profile=uwv-employee-deskundigenoordeel)

2) User advances to skeleton and confirms the UWV answer structure.
   - API: Advance(SKELETON)
   - Artifact: MessageSkeleton (sections aligned to UWV questions)

3) User advances to evidence needs and checks statutory docs/gaps.
   - API: Advance(EVIDENCE_NEEDS)
   - Artifact: EvidenceNeeds (e.g., Plan van Aanpak missing)

4) User binds evidence and reviews missing items.
   - API: Advance(EVIDENCE_BINDING)
   - Artifact: EvidenceSet + EvidenceNeedMap

5) User resolves spans and generates draft.
   - API: ResolveSpans → Advance(SPAN_RESOLUTION) → Advance(DRAFT)

6) User validates, then iterates or exports.
   - API: ValidateRun
   - Output: Markdown draft (PDF later)

## 10) pi adapter (library contract)

Location: libs/pi (shared library)
Defaults:
- Model: Opus 4.5
- Thinking: max
- Credential lookup: same paths as clawdbot (see libs/pi/README.md)

Usage:
- Prepare prompt with explicit “free‑text only” instruction
- Pass context as plain text blocks (no IDs generated by LLM)
- Validate output for structure (presence of sections), not content

## 11) System interaction flow (sequence)

```
User/UI → Orchestrator: CreateRun(profile)
Orchestrator → FS: load intent_seed_path
Orchestrator → pi: generate MessageSkeleton
Orchestrator → pi: generate EvidenceNeeds
Orchestrator → CasePipe: export/query evidence
Orchestrator → LawBot: query legal sources
Orchestrator → LogicGraph: ResolveSpans
Orchestrator → pi: generate DraftText
Orchestrator → UI: GetRun/ListArtifacts/GetArtifact
```

## 12) API call flow (detailed)

Create run
- Request: CreateRun(profile)
- Response: RunContext (state = INTENT_SEED; artifacts include IntentSeed + KeyEvidencePins)

Advance → SKELETON
- Preconditions: IntentSeed exists
- Response: MessageSkeleton artifact

Advance → EVIDENCE_NEEDS
- Preconditions: MessageSkeleton exists
- Response: EvidenceNeeds artifact

Advance → EVIDENCE_BINDING
- Preconditions: EvidenceNeeds exists
- Internal calls: CasePipe + LawBot adapters
- Response: EvidenceSet + EvidenceNeedMap

ResolveSpans
- Request: ResolveSpans(run_id, locators)
- Response: SpanRef[]
- Then: Advance → SPAN_RESOLUTION

Advance → DRAFT
- Preconditions: SpanRef + CitationLedger exist
- Response: DraftText + CitationLedger

Validate
- Request: ValidateRun(run_id)
- Response: errors[] (empty if valid)

## 12.1) Day‑1 vs later UX outcomes

Day‑1 (minimum viable, still useful):
- User can run a profile, inspect artifacts, and export Markdown draft with citations
- UI supports read‑only navigation; CLI mirrors API calls
- Outcome: user has a defensible, cited Markdown draft they can send or edit

Later (after behavior is validated):
- PDF materialization from Markdown
- UI editing of intent + evidence pins (without breaking determinism)
- Outcome: user can generate polished PDF while preserving citation integrity

## 12.2) Adapter contract shapes (owned stack)

CasePipe adapter I/O (conceptual):
- Request: { run_id, query, filters, time_range, strand_id, thread_ids, doc_ids }
- Response: EvidenceRef[] (CasePipe ids) + minimal metadata

LawBot adapter I/O (conceptual):
- Request: { run_id, query, jurisdiction, citation_policy }
- Response: EvidenceRef[] (statute/case ids) + citations

These are internal contracts; we own both ends and will formalize into protobuf
after OrchestratorService stabilizes.

## 12.2.1) Adapter protobuf contracts (explicit)

Adapter protos live in: proto/lawbot/orchestrator/v1/adapters.proto
These are first‑class and should be implemented alongside OrchestratorService.

## 12.3) Example API payloads (textproto)

CreateRun request (textproto):
```
profile {
  profile_id: "example-2025-12-09-forensic-reply"
  intent_seed_path: "docs/user_intent/2025-12-09-email/seed_points.md"
  key_evidence_pins { source_type: EVIDENCE_SOURCE_TYPE_CASEPIPE source_id: "doc:sha256:e7013685388953fe51dc11bb432ebcee33d31c69658884dd7f11bbcc19a1245f" }
  output_type: "forensic email reply"
  tone_goal: "patio11 dangerous professional"
  required_sections: "Written commitments and record-keeping"
}
```

Advance request (textproto):
```
run_id: "run_abc123"
target: ORCHESTRATOR_STATE_SKELETON
```

## 13) Outputs (formats and materialization)

Primary output: Markdown (DraftText.content).  
Later materialization: PDF (HTML‑based or LaTeX‑based, whichever yields better typography).  
Markdown is “good enough” for v1; PDF is a follow‑on step after validation.

## 14) Determinism and validation (ZFC enforcement)

Determinism:
- Evidence lists sorted by (source_type, source_id)
- Artifact ids are stable hashes of (profile_id, state, version)
- SpanRef.excerpt_sha256 must match excerpt

ValidateRun fails if:
- DraftText references statements not found in CitationLedger
- CitationLedger references spans not resolvable by SpanRef
- EvidenceRef.source_id is missing or unknown

Additional validation rules:
- EvidenceRef.source_id must match the explicit ID conventions (see Section 6)
- GetArtifact must return raw protobuf (not JSON/base64)
- Span resolution must be performed by LogicGraph; kernel never infers spans

## 15) Artifact storage (concrete guidance)

Suggested layout (repo‑local MVP):
- .orchestrator/runs/<run_id>/
  - context.pb
  - artifacts/
    - intent_seed_v1.pb
    - message_skeleton_v1.pb
    - evidence_needs_v1.pb
    - evidence_set_v1.pb
    - citation_ledger_v1.pb
    - draft_text_v1.pb

Artifact ids should match filenames for traceability.

## 16) Testing philosophy and trust

Testing pyramid (explicit):
- Unit: deterministic transforms (ids, ordering, hashing, span verification)
- Integration: OrchestratorService API + adapters (CasePipe/LawBot/LogicGraph)
- E2E: end‑to‑end run using a real profile (Dec 9 or UWV) with validated citations

Trust criteria:
- Every citation resolves to a SpanRef and back to evidence
- ValidateRun gates release (fail closed)
- Deterministic outputs across re‑runs with same inputs

Blast radius management:
- Changes to adapters affect evidence binding only
- Changes to kernel affect all stages; require full E2E validation
- Proto changes are versioned; keep backward compatibility within v1

## 17) Incremental delivery plan

Deliver in slices that always produce a usable artifact:
1. pi library (shared LLM adapter) with defaults
2. Kernel + state machine + profile ingestion (prints state, no LLM)
3. IntentSeed + MessageSkeleton generation (LLM only; no evidence)
4. Evidence binding via CasePipe/LawBot adapters
5. Span resolution + citation ledger
6. Draft generation + validation
7. UI read‑only navigation

Each step should end in a “working demo” that produces a user‑visible artifact.

## 18) Implementation order

1. pi library (shared LLM adapter) with defaults
2. Artifact store + RunContext (Go)
3. OrchestratorService RPC server
4. CasePipe adapter (export bundle)
5. LawBot adapter (legal citations)
6. UI read‑only navigation
