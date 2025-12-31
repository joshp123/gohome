# ADR: Orchestrator Kernel (Go, Protobuf-First, ZFC)

Date: 2025-12-29
Status: Proposed

## Context

We are unifying LawBot (legal research), CasePipe (evidence corpus), and
LogicGraph (span/argument utilities) into a single system that produces
defensible legal documents with drillable citations. The workflow must be
iterative and user-first: start with what we want to say, then gather evidence
to support it. The system must be ZFC-compliant: the LLM may only output free
text; evidence pointers and citations must be formal and deterministic.

We control the full stack and do not need legacy/deprecated compatibility
layers in protobuf APIs. Go is the default language for the orchestrator
kernel and new core services.

User intent anchors (canonical workflows):
- 2025-12-09 forensic reply (Spotify email response):
  docs/user_intent/2025-12-09-email/seed_points.md
  docs/user_intent/2025-12-09-email/letter_jp_2025-12-09.md
  docs/user_intent/2025-12-09-email/goals.md
- UWV deskundigenoordeel (employee request about employer reintegration):
  docs/user_intent/uwv/deskundigenoordeel_workflow.md

## Decision

1) Build a Go-first orchestration kernel driven by a formal state machine.

2) Use protobuf as the single source of truth for all interfaces and artifacts.

3) Invert the workflow: intent and key evidence pins precede evidence discovery.

4) Use a shared, batteries-included pi library for LLM calls; it only emits
free text artifacts.

5) Use LogicGraph span semantics (SpanLocator -> SpanRef) to resolve evidence
quotes deterministically.

6) Frontend ↔ backend must use protobuf over the wire (no JSON on the API path)
   whenever possible.

## Workflow profiles (agent-ready)

A workflow profile is the minimum set of inputs needed to run the orchestration
pipeline without additional steering. It binds intent, tone, and evidence pins.

Profile fields (minimum):
- profile_id (string)
- intent_seed_path (free text)
- key_evidence_pins (formal ids; can be empty at start)
- output_type (free text; e.g., “forensic email reply”, “UWV memo”)
- tone_goal (free text; e.g., “patio11 dangerous professional”)
- required_sections (free text bullets)

Profiles are stored as files under docs/user_intent/ and are treated as
first-class inputs to the kernel. The orchestrator reads these to establish
the initial state.

## State machine (explicit)

States (minimum):
- INTENT_SEED
- SKELETON
- EVIDENCE_NEEDS
- EVIDENCE_BINDING
- SPAN_RESOLUTION
- DRAFT

Allowed transitions:
- INTENT_SEED -> SKELETON
- SKELETON -> EVIDENCE_NEEDS
- EVIDENCE_NEEDS -> EVIDENCE_BINDING
- EVIDENCE_BINDING -> SPAN_RESOLUTION
- SPAN_RESOLUTION -> DRAFT

Allowed rewind targets:
- SKELETON, EVIDENCE_NEEDS, EVIDENCE_BINDING, SPAN_RESOLUTION

Rewind semantics:
- Rewind does not delete prior artifacts; it creates a new version chain.
- Replays must reuse prior artifacts unless invalidated by explicit changes.

## Artifacts (names are mandatory)

- IntentSeed
- KeyEvidencePins
- MessageSkeleton
- EvidenceNeeds
- EvidenceSet
- EvidenceNeedMap
- SpanLocator
- SpanRef
- CitationLedger
- DraftText

Each artifact must have:
- id, version, created_at, provenance, and source_refs (formal ids).

## Workflow profile example (stub)

The following stub is the minimum profile format to run without extra steering.
The storage format is a protobuf message serialized to disk:
proto/lawbot/orchestrator/v1/profile.proto

```
profile_id: "example-2025-12-09-forensic-reply"
intent_seed_path: "docs/user_intent/2025-12-09-email/seed_points.md"
key_evidence_pins:
  - "doc:sha256:e7013685388953fe51dc11bb432ebcee33d31c69658884dd7f11bbcc19a1245f"
output_type: "forensic email reply"
tone_goal: "patio11 dangerous professional"
required_sections:
  - "Written commitments and record-keeping"
  - "Severance risk envelope"
  - "Mediation stance"
  - "7:653a BW non-compete refutation"
```

## Workflow (intent-first with iterative loops)

Stage A: Intent + Key Evidence Pins
- Free text: IntentSeed (user seed outline)
- Formal: KeyEvidencePins (known items/doc ids like the email being replied to)

Stage B: Message Skeleton (free text)
- LLM proposes structure, sections, and intended points

Stage C: Evidence Needs (free text -> formal mapping later)
- LLM emits evidence needs in free text

Stage D: Evidence Binding (formal)
- Kernel queries CasePipe/LawBot deterministically
- EvidenceSet and EvidenceNeedMap are formal ids only

Stage E: Span Resolution (formal)
- Resolve quotes using LogicGraph span semantics

Stage F: Draft + Citations (free text + formal)
- LLM drafts with CitationLedger referencing SpanRefs

Iteration: Explicit rewind targets between B/C/D/E/F with artifact versioning.

## Iteration contract (fast workflow tweaks)

- Every stage output is versioned and immutable.
- Rewind is allowed to any earlier stage without losing history.
- Re-run must reuse prior artifacts unless invalidated by explicit changes.
- Kernel must support targeted replay (e.g., only rebind evidence or only
  regenerate draft) without recomputing earlier stages.

## Interfaces (protobuf-first)

- CasePipe export: deterministic corpus bundle (items, docs, events, threads)
- LawBot query: legal sources + citations
- LogicGraph spans: SpanLocator and SpanRef
- Orchestrator state: run context, artifacts, history, errors, transitions
- Frontend: protobuf API responses, no legacy JSON

## ZFC Compliance

- LLM outputs free text only (IntentSeed, Skeleton, Needs, Draft).
- Kernel stores and validates all formal pointers (EvidenceSet, SpanRefs, Ledger).
- No heuristic inference in the kernel; only deterministic validation.

## Diagram

IntentSeed + KeyEvidencePins
          |
          v
   MessageSkeleton
          |
          v
     EvidenceNeeds <----+
          |             |
          v             |
     EvidenceBinding ---+
          |
          v
      SpanResolution
          |
          v
     Draft + Ledger

## Alternatives Considered

1) Evidence-first workflow
   - Rejected: forces users to gather evidence before deciding what to say.

2) JSON APIs for speed
   - Rejected: lacks strict contracts and encourages drift.

3) Claim ledger as mandatory
   - Deferred: likely overkill for early workflows; may be introduced later.

## Consequences

- All services must adopt protobuf interfaces.
- Orchestrator is Go-first; Python work is treated as a prototype reference.
- Iterative loop design enables fast workflow tweaks without refactoring.
