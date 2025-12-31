# ADR: Doc + Email Text Extraction Pipeline (Clean-Only API)

Date: 2025-12-30
Status: Proposed

## Context

Lawbot‑Vault ingests documents and emails that must be usable by downstream
tools (orchestrator, evidence bundles, exports). Today, extracted text quality
is inconsistent and extraction is interleaved with ingestion. We need a
deterministic, repeatable post‑import extraction step that produces a clean
canonical text view while preserving raw outputs for auditability. The API
should expose only cleaned text to avoid leaking noisy raw extractions.

## Decision

1) Add a **post‑import extraction pipeline** for docs and emails.
2) Store **raw** and **clean** text variants (deterministic transforms only).
3) Expose **clean text only** via API responses.
4) Version all transforms and make extraction **idempotent and re‑runnable**.
5) Disallow LLMs or heuristic scoring in extraction (ZFC compliance).

## Alternatives Considered

1) Extract during ingest only.
   - Rejected: blocks iteration and makes cleanup changes risky.

2) Expose raw extraction in API.
   - Rejected: pushes noise to downstream tools and UIs.

3) Use LLM cleanup for formatting.
   - Rejected: violates ZFC; introduces nondeterminism.

## Consequences

- New extraction step becomes part of the repeatable pipeline.
- DB needs to store raw + clean variants and transform metadata.
- API contracts will be updated to surface only clean text for docs/emails.
- Downstream tools can treat clean text as canonical evidence.
