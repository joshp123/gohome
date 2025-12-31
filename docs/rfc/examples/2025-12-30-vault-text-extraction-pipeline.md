# RFC: Vault Text Extraction + Clean Canonical Output

- Date: 2025-12-30
- Status: Accepted
- Audience: Lawbot‑Vault backend, UI, orchestrator tooling, agents

## 1) Narrative: what we are building and why

We need deterministic, auditable text extraction for documents and emails so
Lawbot‑Vault can provide canonical evidence to downstream tools (orchestrator,
exports, evidence packs). The current state is inconsistent: some PDFs have no
extracted text, and markitdown output is noisy (page breaks, broken lists,
soft‑wrapped lines). This RFC defines a **post‑import** extraction pipeline that
produces clean canonical text (exposed via API) while preserving raw outputs
internally for auditability.

This directly supports the Lawbot‑Hub north star: defensible legal documents
with drillable evidence. Clean, deterministic text is required for reliable
citations and re‑use in future workflows.

## 1.1) Non‑negotiables

- ZFC‑compliant: **no LLMs**, no heuristic scoring, no semantic inference.
- Deterministic transforms only.
- Post‑import (re‑runnable) pipeline; **never** mutate source files.
- API exposes **clean text only**.

## 2) Goals / Non‑goals

Goals:
- Provide canonical clean text for docs + emails.
- Make extraction idempotent and versioned.
- Keep raw extraction available for audit/debug.
- Allow incremental improvements to cleaning without re‑ingest.

Non‑goals:
- OCR improvements (separate pipeline).
- UI redesign (later).
- Semantic cleanup or ranking.

## 3) System overview

Pipeline stages:
1) **Ingest**: docs/emails inserted into DB + storage.
2) **Extract** (new): produce raw + clean text variants.
3) **Chunk/FTS**: index clean text for search and downstream tools.

## 4) Components and responsibilities

- `vaultctl docs-extract`: extract + clean doc text (markitdown → clean).
- `vaultctl attachments-extract`: extract + clean attachment text (markitdown → clean).
- `vaultctl email-pretransform`: clean email text variants post‑import.
- Storage layer: persist raw + clean variants with transform versions.
- API layer: return clean text only.

## 5) Inputs / workflow profiles

Docs extraction inputs:
- `--out` (vault_out dir, required)
- `--doc-type` (optional filter, default `pdf`)
- `--source` (optional, default `filesystem`)
- `--limit`, `--offset` (optional)
- `--force` (re‑compute even if clean text exists)
- `--raw-chunk-version` (optional; default `doc_raw_md_v1`)
- `--chunk-version` (clean version; default `doc_clean_v1`)

Validation:
- Doc exists in `docs` table.
- Extraction is skipped if clean text with the same `chunk_version` already
  exists unless `--force`.

## 6) Artifacts / outputs

Per doc:
- **raw_markdown**: direct markitdown output (stored in `chunks` with
  `chunk_version=doc_raw_md_v1`).
- **clean_text**: deterministic cleanup of raw_markdown (stored in `chunks`
  with `chunk_version=doc_clean_v1`).

Per attachment:
- **raw_markdown**: stored in `chunks` with `chunk_version=pdf_markitdown_raw_v1`.
- **clean_text**: stored in `chunks` with `chunk_version=pdf_markitdown_clean_v1`.

Emails:
- `email-pretransform` already stores `clean_text` / `clean_markdown` in `bodies`
  with `transform_version=pretransform_v1`.

Run artifacts:
- `docs_extract_report_*.json` and `attachments_extract_report_*.json`
  with counts + failures.

## 6.1) Schema usage (no new tables)

Docs:
- Reuse `chunks` table.
- Raw and clean variants are distinguished by `chunk_version`.
- FTS is rebuilt **only** for clean versions.

Emails:
- Reuse existing `bodies` table (`clean_text`, `clean_markdown`, `render_html`).

## 7) State machine (if applicable)

Not a state machine; deterministic batch pipeline:

```
ingest -> extract(raw) -> clean -> chunk/fts
```

## 8) API surface (protobuf)

Docs:
- `/api/v3/docs/{doc_uid}` returns:
  - `extracted_text_preview` (clean only)
  - `extracted_text_preview_truncated` (bool)
  - `extracted_text_preview_limit_chars` (int)
- `/api/v3/docs/{doc_uid}/text` returns **full clean text** (paginated):
  - query: `offset` + `limit` (characters)
  - response: `text`, `offset`, `limit`, `total_chars`, `truncated`

Emails:
- Threads API returns:
  - `body_markdown` (renderable)
  - `body_html` / `body_html_raw`
  - Clean text is stored in `bodies` and can be surfaced later as needed.

Raw text is **not exposed** via API.

## 9) Interaction model

- CLI:
  - `vaultctl docs-extract --out ~/.lawbot/corpora/lawbot-vault/vault_out`
  - `vaultctl attachments-extract --out ~/.lawbot/corpora/lawbot-vault/vault_out`
  - `vaultctl email-pretransform --out ~/.lawbot/corpora/lawbot-vault/vault_out --config <dir> --run-id <id>`
- UI reads clean text via existing `/api/v3` endpoints.

## 10) Determinism and validation

Deterministic cleanup rules (clean‑v1):
- Normalize line endings (CRLF → LF).
- Collapse 3+ blank lines to 2.
- Join soft‑wrapped lines within a paragraph.
- De‑hyphenate line breaks (`exam-\nple` → `example`).
- Preserve list markers (`-`, `*`, `•`, `1.`).
- Replace form‑feed (`\f`) with `--- PAGE ---` when `--page-markers` is set; otherwise remove.

Validation:
- Failures recorded in run report.
- Extremely short outputs flagged in report (no blocking).
- If `clean_text` is empty but `raw_markdown` exists, mark as `needs_review`.

## 11) Outputs and materialization

Primary output: **clean text** (canonical for API + search).
Secondary: raw_markdown retained for audit/debug only.

## 12) Testing philosophy and trust

- Golden‑doc tests for extraction + cleanup.
- Coverage checks: % of PDFs with clean text > 0.
- Spot‑check via UI for a small set of PDFs.
- Runtime budget: single run should finish within minutes for a few hundred PDFs;
  report must include wall‑clock time and counts.

## 13) Incremental delivery plan

1) Implement `docs-extract` (markitdown → clean → chunks).
2) Update `attachments-extract` to output clean chunks.
3) Rebuild FTS from clean versions only.
4) Backfill current corpus.
5) Add tests (golden docs + coverage).

## 14) Brutal self‑review (required)

Junior engineer:
Resolved: explicit chunk_versioning and run reports; no schema churn.

Mid‑level engineer:
Resolved: FTS rebuilt only for clean versions; raw never indexed.

Senior/principal engineer:
Resolved: canonical text is clean-only; raw retained in chunks for audit but never served.
Rollback: re‑run extract with earlier clean version and rebuild FTS.

PM:
Resolved: user‑visible clean text in docs view + improved search readiness.

EM:
Resolved: deterministic pipeline; rerunnable; can be forced for upgrades.

External stakeholder:
Resolved: clean text is canonical evidence; raw available for audit.

End user:
Resolved: cleaner evidence text, less hallucination risk in downstream tools.
