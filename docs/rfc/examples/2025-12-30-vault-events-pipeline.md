# RFC: Lawbot‑Vault Events Pipeline (ZFC‑First, Deterministic)

- Date: 2025-12-30
- Status: Draft
- Audience: engineers + operators (pipeline + UI + orchestrator)

## 1) Narrative: why we’re doing this (user perspective)

Events are the **case timeline**. They anchor:
- lawbot‑orchestrator outputs (memos, emails, formal replies),
- evidence packs for UWV,
- and any future narrative‑driven legal artifacts.

Because these events may appear in formal documents—and potentially in front of
a court—**accuracy and provenance are non‑negotiable**. The system must be
machine‑readable so other LLM‑driven tools can interpret a coherent case narrative
without hallucination or misclassification.

This raises the stakes: any heuristic or implicit classification here risks
polluting the entire case story. Therefore we require a ZFC‑clean, deterministic
core that keeps only *high‑signal* events and makes provenance explicit.

We start with the **latest curated upstream events set** (calendar review +
known high‑signal events). It’s “okay” but acceptable. We do *not* expand
scope until the curated pipeline is stable.

## 2) Non‑negotiables (ZFC boundary)

- **No heuristic ranking/scoring** inside ingest.
- **No automatic promotion** from invites/suggestions into canonical events.
- **No keyword/regex classification** for relevance; only curated inputs.
- **No LLM writes to canonical tables** without explicit approval + review.
- **Deterministic transforms only** (schema validation, normalization, hashing).

If it requires judgment, it must be **explicitly curated** outside ingest and
then imported as data.

## 3) High‑signal vs. low‑signal (scope guardrail)

**High‑signal events (in scope now):**
- Company doctor appointments (very high signal). Authoritative source: invite email.
- HSK appointments (high signal). Source: personal calendar (no obvious emails).
- Meetings initiated by the company sent to personal email (very high signal).
  (kind: `meeting_company`)
  These are case‑relevant company meetings, not general work calendar noise.
- 2e spoor meetings (Helena) (high signal). Authoritative source: invite email.
- Arbeidsdeskundige meetings (high signal). Authoritative source: invite email.
- First sick day (very high signal). Source: Slack screenshot in evidence bundle.

**Low‑signal / out of scope (for now):**
- General company emails used as timeline items (future).
- Company calendar invites not clearly tied to the case (future).
- Sick‑leave milestones (future; representation unclear).
- Events derived heuristically from email text (future, only via reviewed pipeline).

This RFC only covers the **personal, high‑signal events** above.

### 3.2 Formality requirement (titles and wording)

Every canonical event must have a **clear, formal title** suitable for legal
documents (e.g., “Company doctor appointment”, “Arbeidsdeskundige meeting”).
Titles must be human‑curated and should stand on their own when cited later.

### 3.1 High‑signal provenance expectations (explicit)

For each high‑signal event type, the canonical provenance chain must be explicit
and **curated** (no heuristics):

- **Company doctor appointments** → invite email(s) + reminders/reschedules
  (authoritative) + doctor report docs (linked after review).
- **HSK appointments** → personal calendar export (authoritative) + optional
  confirmations (if any).
- **Company meetings sent to personal email** → invite email(s) (authoritative).
- **2e spoor (Helena) meetings** → invite email(s) (authoritative).
- **Arbeidsdeskundige meetings** → invite email(s) (authoritative).
- **First sick day** → Slack screenshot doc in evidence bundle (authoritative).

Reminders/reschedules are captured in a **single provenance chain** and collapsed
into one canonical event; the chain is hidden in UI by default but remains auditable.

## 4) Inputs (current scope)

### 4.1 Curated events manifest (REQUIRED)

Canonical events are created **only** from a curated manifest. This prevents
heuristic creep and makes provenance explicit.

**Location:** hidden app data (not committed):
```
~/.lawbot/config/lawbot-vault/events/curated_events_v1.json
```
A template can live in‑repo (e.g., `docs/rfc/templates/curated_events_v1.template.json`),
but the real data stays **outside git**.

### 4.1.1 Curated export snapshot (for review)

After each import, produce a **curated export snapshot** for human review. This
is a small, machine‑readable summary used to confirm quality, not a replacement
for the manifest.

Minimum contents:
- counts by kind
- list of events with: `event_uid`, `title`, `occurred_start`
- authoritative invite emails (item_uids) and report docs (doc_uids)
- any missing‑provenance exceptions (`no_invite_reason`, `no_report_reason`)

**Schema (required fields marked REQUIRED):**
```
{
  "schema_version": 1,
  "source_label": "curated events v1",
  "events": [
    {
      "event_uid": "event:canon:company_doctor:2024-09-04:call",  // REQUIRED stable canonical id
      "kind": "company_doctor",                                   // REQUIRED
      "title": "Company doctor",
      "occurred_start": "2024-09-04T08:35:00Z",                 // REQUIRED
      "occurred_end": "2024-09-04T09:00:00Z",
      "authoritative_item_uids": ["item:..."],              // invite emails (authoritative)
      "supporting_item_uids": ["item:..."],                 // reminders/reschedules
      "authoritative_doc_uids": ["doc:sha256:..."],         // e.g. first sick day screenshot
      "supporting_doc_uids": ["doc:sha256:..."],
      "notes": "why this is high signal",
      "no_invite_reason": "optional override if no email exists",
      "no_report_reason": "optional override if no report doc exists",
      "allow_duplicate": false                               // optional explicit override
    }
  ]
}
```

**Validation rules (import must fail if violated):**
- `source_label` required (non‑empty).
- `event_uid` required and unique within the manifest.
- `event_uid` must be stable and non‑derived from heuristics (explicit in manifest).
- `event_uid` format (required): `event:canon:<kind>:YYYY-MM-DD[:slug]`
  - example: `event:canon:company_doctor:2024-09-04:call`
  - optional `:slug` is required when multiple same‑day events exist.
- `title` is required and must be non‑empty. Formal wording is enforced by
  operator review (not by code heuristics).
- `kind` must be one of the **allowed kinds** below (others require RFC change):
  - **In‑scope kinds:** `company_doctor | hsk | meeting_company | 2e_spoor |
    arbeidsdeskundige | first_sick_day`
  - **Legacy kinds (allowed for upstream parity, not expanded):**
    `lawyers | reintegration | milestone`
- `occurred_start` must be RFC3339.
- `event_uid` date (YYYY‑MM‑DD) must match `occurred_start` date (UTC).
- High‑signal kinds require provenance:
  - company_doctor / 2e_spoor / arbeidsdeskundige / meeting_company →
    at least one `authoritative_item_uids` OR `no_invite_reason`.
  - company_doctor → at least one `authoritative_doc_uids` OR `no_report_reason`.
  - first_sick_day → at least one `authoritative_doc_uids`.
- `allow_duplicate` defaults to false.
- Unless `allow_duplicate=true`, there must be **at most one** event per
  `<kind, occurred_date>` in the manifest. If duplicates exist, each must have a
  unique `:slug` suffix and `allow_duplicate=true`.
- All referenced `item_uid`/`doc_uid` must exist in the target DB at import time.
  (If missing, import fails; ingest emails/docs first.)
- `authoritative_*` and `supporting_*` lists must not overlap (no duplicates).

**Ordering semantics (explicit):**
- `authoritative_item_uids[0]` must be the **latest** invite version.
- `supporting_item_uids` must include reminders/reschedules in any order (stable,
  but order has no semantic meaning beyond audit convenience).
- If an invite changes, the **newest** invite moves to `authoritative_item_uids[0]`
  and the older invite is retained in `supporting_item_uids`.

### 4.2 Calendar‑review JSON (curated, but secondary)

Calendar‑review JSON can be used as *inputs* to build the curated manifest, but
it is **not** canonical by itself. Only the curated manifest produces events.
Any conversion from calendar‑review JSON to the manifest is **manual / curated**
(no heuristics, no auto‑merge by UID/subject).

### 4.3 Email calendar invites (non‑canonical)

Parsed from `text/calendar` parts / `.ics` attachments and stored in
`calendar_invites`. These are **review artifacts**, not canonical events.

## 5) Canonical vs. suggestions (hard boundary)

- **Canonical events** live in `events` and are sourced only from the curated manifest.
- **Non‑canonical artifacts** (calendar invites, candidate suggestions) live in
  separate tables (`calendar_invites`, future `event_suggestions`).
- **No auto‑merging or ranking** across sources.

## 6) Event IDs + reschedule rules

Canonical IDs are provided by the curated manifest only. Reschedules/reminders
must not create new canonical events; they extend provenance on the same event.

**Reschedule policy (explicit, no heuristics):**
- If a meeting time changes, the curator adds the new invite email to
  `supporting_item_uids` for the *same* `event_uid`.
- No automatic merging by `ical_uid` or subject text.
- If two events share an `ical_uid`, they remain separate unless the curator
  explicitly merges them in the manifest.
- If a duplicate is truly required, `allow_duplicate=true` must be set.

## 7) Provenance rules (sharp + auditable)

Every canonical event must carry provenance:
- source label (human string)
- source file path (manifest path)
- import run report id
- authoritative source chain (invite emails, reminders, reschedules)
- doc link targets (when known)

**Implementation constraint (ZFC‑safe):**
- Provenance chains are supplied by the curated manifest (explicit mapping from
  `event_uid` → `item_uid[]` / `doc_uid[]`). No inference.

## 8) Import behavior (deterministic)

### 8.1 Curated events import
- Validate required fields and rules above.
- Normalize timestamps to UTC seconds.
- Insert `events` (idempotent by `event_uid`).
- Insert `event_items` for `authoritative_item_uids` and `supporting_item_uids`.
- Insert `event_docs` for doc links (authoritative/supporting).
- Store provenance metadata in `description` + `participants_json` (mechanical).
- **Do not compute or assign `importance_score`.**
- Write run report (counts, source path, timestamp).

**Manifest discovery (explicit):**
- CLI flag: `--manifest /path/to/curated_events_v1.json`
- Default path (if flag omitted): `~/.lawbot/config/lawbot-vault/events/curated_events_v1.json`

### 8.2 Calendar invites from email
- Parse `.ics` only into `calendar_invites`.
- Keep series UID and recurrence fields.
- Do **not** create `events` rows from invites.

## 9) Duplicate handling (explicit only)

- No automatic de‑dupe across sources.
- No “similarity” merges.
- Duplicates are resolved only by:
  1) curated manifest (explicit merge or allow_duplicate=true), or
  2) explicit operator action (future UI), or
  3) stable ID collision (same `event_uid` → idempotent skip).

## 10) Comparison with prior exports (required review)

We must compare with the old DB exports (calendar review + invites) to ensure:
- High‑signal events exist and are correctly represented.
- Provenance is sharper (invite chain visible, duplicates collapsed).
- Noise is reduced (calendar spam excluded).

Any divergence from the curated upstream set must be explicitly justified.

**Observed in old DB (baseline facts):**
- `events`: 33 total
  - kinds: `company_doctor` (12), `hsk` (16), `lawyers` (2), `2e_spoor` (1),
    `reintegration` (1), `milestone` (1)
- `calendar_invites`: 274 total, with duplicate `ical_uid` entries in the work
  mailbox (recurring series → collisions).
- `event_items`: 0 (no direct link to invite emails)
- `event_docs`: 44 (most events point to the same calendar export doc)
- canonical provenance often shows `canonical_source_type = calendar_export`
  instead of invite emails.

**Comparison checklist (must pass):**
- High‑signal events exist in curated manifest for each kind in scope.
- Each company doctor / 2e spoor / arbeidsdeskundige / meeting_company event has
  at least one authoritative invite email (or explicit `no_invite_reason`).
- First sick day event has authoritative doc uid (Slack screenshot).
- `event_items` is **non‑zero** after import (proves invite linkage exists).
- Company doctor events link at least one report doc (or explicit `no_report_reason`).
- No duplicate events per kind+date unless `allow_duplicate=true`.

## 11) Why the old pipeline failed (explicitly)

From prior DB + history:
- **Weak provenance**: events point to a calendar export doc, not the invite email.
- **Invite collisions/duplicates**: recurring `ical_uid` series in work calendar
  produced duplicates and noise.
- **Heuristic creep** (“retarded parts” / “slopficiation” feedback): ad‑hoc rules
  guessed relevance and links, producing low signal.

Additional failure mode observed:
- **No invite linkage at all** (`event_items` = 0), so “authoritative source” was
  never captured even when invite emails existed.

This RFC avoids those failures by: (1) curated‑only canonical inputs, (2) explicit
provenance mapping, and (3) no heuristic scoring or auto‑promotion.

## 12) Acceptance criteria (phase 1)

- Importing the curated manifest is **idempotent**.
- Canonical events **only** originate from curated input.
- Calendar invites remain **non‑canonical**.
- No `importance_score` is computed by ingest.
- Run report produced for every import.
- For high‑signal event kinds, required provenance exists:
  - company_doctor / 2e_spoor / arbeidsdeskundige / meeting_company →
    at least one authoritative invite email OR `no_invite_reason`.
  - company_doctor → at least one report doc OR `no_report_reason`.
  - first sick day → authoritative doc uid present.
- If any event violates the validation rules, the import fails fast.

## 13) Operator workflow (explicit)

**Driver:** the implementation agent drives the curation + import loop and
shares curated exports with the user for review (as done for emails/docs).
The user approves quality; only then is the manifest considered canonical.

Workflow:
1) Ensure emails + docs are ingested (item_uids/doc_uids must exist).
2) Curate events in the manifest (`curated_events_v1.json`), using only
   explicit evidence links and formal titles.
3) Run `vaultctl events-import-curated --manifest <path>` (TBD) to import.
4) Produce a curated export snapshot for review (counts, sample events,
   provenance chain coverage + any missing‑provenance exceptions).
5) Compare counts + provenance vs old DB using the checklist above.
6) Iterate manifest until acceptance criteria pass and user signs off.

## 14) Future (explicitly gated)

- Series‑level calendar invite triage UI + rules table.
- LLM suggestion pipeline with human review + apply step.
- Deterministic report‑to‑event linkers (post‑review).
- Link events to docs and invite emails (shown as provenance; hidden by default).
- Timeline view that includes key email threads (severance, GDPR).
- Sick‑leave milestones (when representation becomes clear).
