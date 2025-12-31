# RFC: Lawbot‑Vault Email Selection + Ingest Pipeline (Deterministic, Modular)

- Date: 2025-12-30
- Status: Draft
- Audience: engineers + operators (pipeline + UI + orchestrator)

## 1) Why this exists

Email ingest defines the **canonical evidence corpus**. It decides what becomes
queryable, citeable evidence for the orchestrator. We therefore need a
deterministic, auditable, and modular pipeline that can be re‑run safely as new
emails arrive or selection rules evolve.

This RFC specifies **selection philosophy** and **pipeline boundaries**, aligned
to the orchestrator RFC (`docs/rfc/2025-12-29-orchestrator-spec.md`).

## 2) Principles

1) **Deterministic:** given the same mailbox state + config, outputs are identical.  
2) **Auditable:** every decision has a reason code and is inspectable.  
3) **Modular:** select/extract/attachments/pretransform/fts are independent steps.  
4) **Minimal noise:** exclude irrelevant work‑calendar and listserv noise by default.  
5) **Separation of concerns:** ingest is deterministic; LLM is post‑ingest only.  

## 3) Orchestrator alignment (EvidenceRef contract)

Email evidence IDs must match orchestrator requirements:

```
EvidenceRef.source_id = doc:email:<item_uid>
item_uid = <source_id>:<source_rowid>
```

This is stable across re‑runs as long as selection references the same mailbox
rowids.

## 4) Pipeline stages (modular CLI)

Each stage is a **standalone** CLI, and `email-ingest` orchestrates them:

1. **Select** (`email-select`)  
   - Evaluate selection rules and write `selections` + `items`.
2. **Extract** (`email-extract`)  
   - Parse `.emlx` → `raw_rfc822`, `source_text`, `source_html`, participants, calendar_invites.
3. **Attachments** (`email-attachments`)  
   - Persist attachment files + rows; recover from Mail.app attachment store.
4. **Pretransform** (`email-pretransform`)  
   - Produce `clean_text`, `clean_markdown`, `render_html`.
5. **FTS** (`email-fts`)  
   - Rebuild search index over clean bodies.

Post‑ingest (explicit, optional):
- **Attachment text extraction** using `markitdown` (PDFs, etc).
- **Event suggestions** from emails (separate runner, off by default).

## 5) Selection rules (order + behavior)

Selection is the **most critical** stage because it defines the corpus.

Order of evaluation:
1) **Rowid allow/deny lists** (deny wins).  
2) **Mailbox scoping** (mandatory; see below).  
3) **Date window** (if configured).  
4) **Address/domain filters**.  
5) **Calendar‑only filtering** (configurable).  
6) **Default include** (only after all filters pass).

### Mandatory mailbox scoping

Each source **must** define `include_mailboxes` to avoid cross‑account
contamination. If missing, **fail fast** with a clear error.

Required for current sources:
- Personal: `include_mailboxes` must include `elliot@ecorp.com`
- Work mirror: `include_mailboxes` must include `tyrell@ecorp.com`

### Calendar‑only handling (default: off)

We do **not** want noise from recurring work invites. We also want the ability
to ingest some invites later.

Default behavior:
- If a message has **no usable body** and only `text/calendar`, it is excluded
  unless explicitly allowed by selector.

Config override:
- `allow_calendar_only: true` on a selector OR explicit include list of senders
  / mailboxes for calendar‑only messages.

This keeps the default corpus clean while preserving an explicit path to include
invites later.

## 6) Extraction outputs (bodies + participants)

We store multiple body variants with explicit semantics:

- `raw_rfc822` (bytes) — canonical raw email payload.  
- `source_text` — best text/plain or HTML‑derived text.  
- `source_html` — best HTML part (as‑received).  
- `clean_text` — deterministic cleaned text for search/LLM.  
- `clean_markdown` — deterministic markdown (for analysis tooling).  
- `render_html` — sanitized HTML for UI display (safe + trimmed).  

### HTML rendering policy

UI should prefer `render_html` (when present). It is derived from `source_html`
using a deterministic sanitizer:
- strip `script/style`, unsafe tags, and tracking pixels
- trim quoted history blocks + signature blocks (best‑effort, deterministic)
- preserve hyperlinks, lists, and tables where safe

Fallback order for UI:
`render_html` → `clean_markdown` → `clean_text`

## 7) Attachments (deterministic + recoverable)

Attachments are saved deterministically with SHA‑based filenames and recorded
in `attachments`. Recovery from Mail.app’s `Attachments/` store is allowed when
missing in `.emlx`.

**Attachment text extraction is post‑ingest** (using `markitdown`), so the core
ingest remains deterministic and fast.

## 8) Event extraction

Email‑derived event extraction is noisy and **not part of deterministic ingest**.
It must be run as a separate, opt‑in post‑process step, with explicit allowlists.

## 9) Run reports + audit

Every stage must write:
- counts (selected, excluded, bodies written, attachments saved, missing)
- warnings/errors (bounded list)
- config hash + run id + timestamps

These reports are evidence of correctness, not “logs”.

## 10) What we deliberately do NOT do

- No LLM calls inside ingest.  
- No auto event extraction inside ingest.  
- No UI‑driven filtering as a substitute for selection rules.  

## 11) Future refactor notes

After parity is achieved in Go:
- Compress or externalize raw bodies to reduce DB size if needed.
- Incremental FTS updates instead of full rebuilds.
- Introduce “suppressed” selection outcomes for noisy classes (instead of exclude).
