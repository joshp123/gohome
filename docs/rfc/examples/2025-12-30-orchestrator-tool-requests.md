# RFC: Orchestrator Tool Requests (User-First, ZFC-Safe Evidence Gathering)

- Date: 2025-12-30
- Status: Draft (needs revision; feedback pending)
- Audience: engineers, PM-minded stakeholders, adjacent teams

## 1) User problem and success outcome (anchor)

User needs a structured, iterative way to assemble legal memos/emails with
traceable citations. The system must let the user define intent, lock structure,
discover evidence needs, bind evidence, resolve exact spans, draft, and rewind any
step. This exists because LLMs cannot one-shot defensible legal writing without
explicit structure and evidence grounding.

Success outcome: the user can reply to company emails or produce UWV
"deskundigenoordeel" memos in one guided flow, with every claim traceable to
primary sources (personal evidence + statutes/caselaw).

## 1.1) Non-negotiables

- ZFC: model never reads evidence sources directly; it only proposes tool requests.
- API-first, protobuf-only (no JSON between components).
- Determinism: tool requests/results are artifacts with stable IDs.
- State-gated tools: allowed tools are explicit per state; violations fail closed.
- Vault is proto-only on the wire (death2json).
- LawBot citations default to statutes only; case law is opt-in as a separate tool.

## 2) Guiding principles (operational)

- Model drives the workflow with sensible defaults.
- User retains trust via clear, staged UI and explicit evidence provenance.
- Evidence is never assumed; it must be retrieved or pinned and then bound.
- The UI must make "what to do next" unambiguous at every step.

## 3) User experience overview (outside-in)

The workflow is intent-first and iterative. At each stage, the user sees:
- a single, clear "Next action" CTA,
- only the inputs relevant to the current stage,
- a live conversation log with the model's guidance,
- a view of evidence pins and tool results.

### Stage-by-stage UX contract

| Stage | User sees | User does | Model does | Tools used | Artifacts |
|------|-----------|-----------|------------|------------|-----------|
| INTENT_SEED | Intent form + "Next action" | Writes intent, tone, sections | Summarizes intent, asks for missing goals | none | IntentSeed, WorkflowProfile |
| SKELETON | Outline panel + approval CTA | Approves/edits outline | Proposes sections + claim list | none | MessageSkeleton |
| EVIDENCE_NEEDS | Evidence checklist + pinned evidence | Pins known items, reviews auto-run results | Proposes missing evidence + tool requests | vault.search, vault.bundle.inspect, lawbot.search (statutes only) | EvidenceNeeds, EvidenceSet |
| EVIDENCE_BINDING | Claims list + evidence candidates | Confirms bindings | Proposes claim->evidence map | vault.resolve_ids, lawbot.resolve_citations | EvidenceNeedMap |
| SPAN_RESOLUTION | Quoted span previews (highlighted exact text + surrounding context) | Confirms exact quotes | Proposes span selections | vault.get_spans, lawbot.get_spans | SpanRef, CitationLedger |
| DRAFT | Draft view w/ citations | Reviews, edits, rewinds | Drafts using bound evidence only | none | DraftText |

### CTA copy map (per stage)

- INTENT_SEED: "Save intent" → "Advance to skeleton"
- SKELETON: "Approve skeleton" → "Advance to evidence needs"
- EVIDENCE_NEEDS: "Review evidence checklist" → "Advance to binding"
- EVIDENCE_BINDING: "Confirm bindings" → "Advance to span resolution"
- SPAN_RESOLUTION: "Confirm quotes" → "Advance to draft"
- DRAFT: "Finalize draft" (or "Rewind")

### Hybrid evidence flow (required)

- The user seeds evidence (pins Vault IDs) when possible.
- The model proposes additional searches when evidence is missing.
- Read-only tool requests auto-run; user confirms pins/bindings.

## 4) System responsibilities (inside-out)

- Orchestrator (Go)
  - Owns the state machine, tool policy, artifact store, validation.
  - Executes tool requests and records ToolResults.
- Model (pi, Opus 4.5 locked)
  - Generates guidance and ToolRequests only.
  - Never claims evidence exists without ToolResults.
- Vault adapter
  - Deterministic search + ID resolution for personal evidence (proto-only transport).
- LawBot-Vault adapter
  - Deterministic search + span resolution for statutes/caselaw.
- UI
  - Shows stage, guidance, suggested tool actions, evidence pins/results.

## 5) Model contract (what the model is allowed to do)

The model has no direct tool access. It may only:
- Propose ToolRequests allowed in the current state.
- Ask the user for missing inputs.
- Draft or summarize only from bound evidence.

Hard guardrails (must appear in the system prompt):
- "Do not claim you searched or found evidence unless a ToolResult exists."
- "If evidence is unknown, ask for a tool request or a user pin."
- "In SKELETON, do not request evidence or binding."

### Guidance output format (strict)

The model must output:
1) Missing inputs (if any)
2) Suggested tool actions (if any)
3) Next action (single CTA)

## 6) Tool policy (auto-run read-only; user confirms binding)

Default policy:
- Read-only tools auto-run when proposed by the model.
- Mutating/ingest tools require explicit user action.
- Auto-run does NOT auto-bind evidence; user confirms pins/bindings.

Read-only tools (auto-run):
- vault.search (kinds filter: THREAD | DOC | EVENT | ALL)
- vault.bundle.inspect
- vault.resolve_ids
- vault.get_spans (personal evidence)
- lawbot.search (statutes only)
- lawbot.resolve_citations (statutes only)
- lawbot.get_spans (statutes only)

Read-only tools (opt-in, manual trigger):
- lawbot.case_search (caselaw)
- lawbot.case_resolve_citations (caselaw)
- lawbot.case_get_spans (caselaw)

Mutating/ingest tools (manual only):
- vault.import
- vault.bundle.attach

If a tool errors or returns empty:
- The model proposes a refined query.
- The UI presents a retry action.

## 7) EvidenceNeeds format (claim-centric, symbiotic)

Evidence needs are claim-centric and grouped by skeleton section. Each claim has
explicit evidence slots with status and candidate evidence from tool results.

### 7.1) EvidenceNeeds proto schema (required)

```proto
message EvidenceNeeds {
  AuditMeta meta = 1;
  repeated EvidenceSection sections = 2;
}

message EvidenceSection {
  string section_id = 1;
  string title = 2;
  repeated EvidenceClaim claims = 3;
}

enum EvidenceSlotStatus {
  SLOT_STATUS_UNSPECIFIED = 0;
  SLOT_STATUS_MISSING = 1;
  SLOT_STATUS_CANDIDATES = 2;
  SLOT_STATUS_BOUND = 3;
}

message EvidenceClaim {
  string claim_id = 1;
  string claim_text = 2;
  repeated EvidenceSlot required = 3;
  repeated EvidenceSlot optional = 4;
  repeated ToolRequest suggested_searches = 5;
}

message EvidenceSlot {
  string slot_id = 1;
  string description = 2;
  EvidenceSlotStatus status = 3;
  repeated EvidenceCandidate candidates = 4;
  repeated EvidenceRef bound = 5;
}
```

### 7.2) Symbiosis rule

Evidence can refine claims and claims can trigger new evidence needs. Evidence →
quote → claim is a two-way loop. The user may rewind to update either.

## 8) UI integration with Vault (embedded + open)

We reuse Vault UI and add a "Select for run" mode:
- Embedded Vault panel inside Orchestrator for quick selection.
- "Open in Vault" link for full context.
- Evidence preview in Orchestrator shows the exact quoted span with a highlight
  and renders the immediate surrounding context as "not included" text.
- Evidence list groups by thread/doc/event with expandable previews and quote lists.

No bespoke evidence browser is built in Orchestrator.

## 9) Standard reference format across the suite

We standardize on LogicGraph span semantics for all tools:

- SpanLocator (quote locator):
  - doc_uid, chunk_uid, exact_quote, occurrence_index, context_before, context_after
- SpanRef (resolved offsets):
  - doc_uid, start_char, end_char, excerpt, excerpt_sha256, locator

This format is used across Orchestrator, Vault, and LawBot.

## 10) Tool output schemas (search results)

We need a standard shape for evidence search results across tools:

```proto
message EvidenceCandidate {
  string evidence_id = 1;     // vault:<doc_uid> or law:<id>
  string thread_id = 2;       // thread grouping for email chains
  string doc_type = 3;        // email | pdf | event | statute | caselaw
  string title = 4;
  string snippet = 5;
  string occurred_at_iso = 6;
  repeated string participants = 7;
  string source_link = 8;     // deep link to Vault/LawBot
}

message EvidenceSearchResult {
  string query = 1;
  repeated EvidenceCandidate candidates = 2;
}
```

These candidates feed EvidenceNeeds and EvidenceSet.

## 10.1) Vault proto endpoints (proto-only, add/extend)

The orchestrator calls Vault via protobuf only. We add two first-class Vault RPCs
so the orchestrator does not fan-out to Threads/Docs/Events itself:

1) **VaultSearchService.Search** (new)
   - Input: query + kinds filter + time range + participants + strand_id
   - Output: EvidenceSearchResult (merged deterministically)

2) **VaultSpanService.ResolveSpans** (new)
   - Input: SpanLocator[]
   - Output: SpanRef[] (LogicGraph semantics)

Existing v3 services remain for UI and deep navigation:
- ThreadsService.ListThreads / GetThread
- DocsService.ListDocs / GetDoc / GetDocText
- EventsService.ListEvents / GetEvent

EvidenceCandidate fields are derived from:
- ThreadSummary (thread_uid, subject, participants, match_snippet)
- DocSummary / DocDetail (doc_uid, title, doc_type, authored_at_iso)
- EventSummary (event_uid, title, occurred_start_iso)

## 11) Missing inputs computation (kernel authoritative)

The kernel computes missing inputs from EvidenceNeeds:
- Each claim has required evidence slots.
- A slot is satisfied if a bound EvidenceRef exists.

Advance is allowed only if all required slots are satisfied.

## 12) Example user flow (Dec 9 reply)

1) User writes intent seed: "Reply to Dec 9 letter; challenge written commitments."
2) User pins the Dec 9 letter from Vault.
3) Model proposes vault.search for the team email; auto-runs.
4) Evidence checklist completes; user advances.
5) Model suggests claim->evidence bindings; user confirms.
6) Model proposes lawbot.search for 7:611 BW; auto-runs.
7) Span resolution selects quotes; user confirms.
8) Draft produced with citations; user edits or rewinds.

## 13) Tool request protocol (protobuf)

Tool requests are first-class artifacts. The model proposes; orchestrator executes.

```proto
message ToolRequest {
  string id = 1;              // toolreq:<ulid>
  string run_id = 2;
  OrchestratorState state = 3;
  string tool_name = 4;       // vault.search, lawbot.search
  ToolRequestParams params = 5;
  bool requires_user_confirm = 6;
  string created_at_iso = 7;
  string requested_by = 8;    // "llm" | "user"
}

message ToolResult {
  string id = 1;              // toolres:<ulid>
  string request_id = 2;
  string run_id = 3;
  ToolResultStatus status = 4; // OK | EMPTY | ERROR
  string summary = 5;
  ArtifactRef output_ref = 6;
  string created_at_iso = 7;
}
```

## 14) API call flow (detailed)

1) Guide
- Request: Guide(run_id, user_prompt)
- Response: ConversationLog + ToolRequestLog

2) Execute tool request (auto-run for read-only)
- Request: ExecuteToolRequest(request_id)
- Response: ToolResultLog + updated EvidenceSet

3) Advance
- Guard: Missing inputs empty
- Response: RunContext

## 15) Determinism and traceability

- ToolRequests and ToolResults are immutable and append-only.
- EvidenceSet changes are traceable to ToolResult IDs.
- DraftText references CitationLedger -> SpanRef -> EvidenceRef.

## 16) Failure UX (explicit)

- Tool result EMPTY: UI shows "No matches" + suggested refined query + retry CTA.
- Tool error: UI shows error + retry CTA + model suggests fallback query.
- Span mismatch: UI highlights the quote and opens an inline floating viewer with
  adjustable context window; user can expand context before confirming.

## 17) Gap analysis (current system vs this RFC)

Resolved:
1) Tool requests auto-run end-to-end (vault.search + lawbot.search) with ToolResult logs.
2) UI gates inputs by stage + surfaces “Next action”.
3) Evidence needs + tool results + candidate pinning are visible in UI.
4) Vault selection mode embedded + “Open in Vault” deep links.
5) VaultSearchService.Search + VaultSpanService.ResolveSpans implemented.
6) LawBot span resolution implemented via proto-only LawBot UI endpoints.

Remaining:
- None. E2E demo captured (Dec 9 reply) with real evidence + citations.

E2E demo evidence:
- Run: `run-20251231T033207Z-acd26c3ed668`
- Draft artifact: `~/.lawbot/runs/run-20251231T033207Z-acd26c3ed668/artifacts/draft_text/v1/4ac48f3b6ee075228e0361ccdd9c4be811b1fc9eea27d312374858c1f2fa5808.pb`
- Citation ledger: `~/.lawbot/runs/run-20251231T033207Z-acd26c3ed668/artifacts/citation_ledger/v1/5f26f43d6c033fcbb83ccf8fcebd431f7ff9f873286f1db9646f721d604c9180.pb`

## 18) Testing philosophy and trust

- Unit: tool policy validation, schema validation.
- Integration: tool execution updates EvidenceSet.
- E2E: run that performs vault search -> binding -> lawbot search -> draft.

## 19) Implementation order (full path to 100%)

1) ✅ VaultSearchService.Search + VaultSpanService.ResolveSpans (proto + server).
2) ✅ EvidenceNeeds proto schema + kernel missing-inputs gating.
3) ✅ ToolRequest/ToolResult storage + auto-run read-only tools.
4) ✅ Guidance prompt includes tool policy + evidence availability.
5) ✅ UI suggested tool actions + stage-gated inputs.
6) ✅ Vault embedded selection mode (postMessage → pinning).
7) ✅ Span confirmation UX (embedded vault preview + excerpt expand).
8) ✅ LawBot span resolution + proto-native adapter.
9) ✅ End-to-end demo run (Dec 9 reply) + tests.
