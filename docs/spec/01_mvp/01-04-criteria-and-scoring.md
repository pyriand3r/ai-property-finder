---
type: spec
created: 2026-07-28
title: Criteria & Transparent Scoring
project: property-finder-obs-style
spec: criteria-and-scoring
phase: 1
order: 4
category: evaluation
status: ready
updated: 2026-08-02
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - evaluation
  - criteria
---

## Overview

Reuses the data model and formula from [[../../dev-property-finder/spec/01_mvp/01-03-criteria-weighting|dev-property-finder]] and [[../../dev-property-finder/spec/01_mvp/01-12-rate-listing-criteria|dev-property-finder]] verbatim: Objective/Subjective Criteria, integer Weight 1–5, A–F Grade per Criterion with reasoning, `score = sum(grade_value * weight) / sum(weight)` mapped A=100…F=0. This spec only covers what's new: how Criteria get created/changed, and direct manual editing.

---

## Criteria Origin

Three ways Criteria enter a profile, all converging on the same `criterion` table:

1. **Initial conversational parse** — first time a Source is ingested into an empty profile, the Agent calls `propose_criteria` derived from the conversation so far (what the user has said they're looking for) plus anything already in the Profile Context. Presented as a confirm card, same pattern as Context proposals ([[01-02-context-files#Confirmation]])
2. **Conversational addition** — "auch nach Nähe zum Wasser bewerten" mid-chat → `propose_criteria` again, this time as a diff against existing Criteria (add/change one, not a full replace)
3. **Direct manual edit** — unlike [[../../dev-property-finder/spec/01_mvp/01-02-ai-parsing-criteria#Resolved Decisions|dev-property-finder's explicit MVP non-goal]], the user can add/edit/remove a Criterion directly in a Criteria list UI on the profile (name, description, Objective/Subjective toggle, Weight slider) without going through the Agent at all

Any Criteria change — add/remove, or edits to name, description, or the Objective/Subjective flag, whether conversational or manual — sets `listing.needs_reevaluation = true` on all Listings in the profile. **Weight-only changes are the exception**: they recompute every Score in pure SQL (no flag, no LLM call) — see Weight below. The user triggers re-evaluation explicitly: per-listing from the detail view, or bulk from the list view banner. No automatic re-evaluation. See [[../../docs/adr/0015-manual-bulk-reevaluation]].

---

## Weight

Unchanged: 1–5 integer, default 1, adjusted via slider, pure-SQL score recompute on Weight-only changes (no LLM call) — see [[../../dev-property-finder/spec/01_mvp/01-03-criteria-weighting#Weight Change vs. Description Change]].

---

## Grade & Reasoning

Unchanged: `evaluate_listing` returns one Grade + reasoning per Criterion, backend maps by position not echoed ID (per [[../../dev-property-finder/docs/adr/0006-llm-response-robustness|ADR 0006]]), full replace on every Evaluation. Displayed in the Listing detail view as the same grid as [[../../dev-property-finder/spec/01_mvp/01-12-rate-listing-criteria#Detail View|dev-property-finder]].

---

## Rejection Feedback, Reimagined

When the user rejects a Listing (via detail form or chat), the backend immediately fires an asynchronous LLM call analysing the rejection comment. This call may produce `propose_criteria` and/or `update_context` proposals. Results appear as Confirm-Cards in the profile's Chat; a toast notifies the user: „Rejection verarbeitet — Vorschläge im Chat verfügbar."

The analysis call is a single-shot LLM request (no Agent loop, no tools) — the same pattern as dev-property-finder's `ParseSearchProfile`, and the same shape this project already uses for `EvaluateListing`/`GenerateRanking` (`internal/ai`, `RejectionAnalysisService`, Layer 8.1). It does not trigger re-evaluation — it only generates proposals. `needs_reevaluation` is set on all listings only after the user confirms a Criteria proposal (via ADR 0015's manual trigger).

Fires only on the transition *into* `rejected` (not on every subsequent save of an already-rejected Listing), and only when a `rejection_comment` is present — a detached goroutine (ADR 0008 fire-and-forget shape), so the PATCH response isn't delayed. If the model finds nothing worth proposing (the common case), nothing is written — no empty chat message, no toast. When it does find something, the proposals are written as one synthetic `chat_message` (`role=assistant`, `tool_calls` populated in the exact same shape a live Agent turn's `propose_criteria`/`update_context` calls would produce) appended to the profile's active session — the existing Confirm-Card UI and `/tool-calls/{idx}/confirm|reject` endpoints (ADR 0009/0017) handle it with zero changes, since as far as that code is concerned it's just another tool call. `chat.message` and `listing.rejection_analyzed` are published on the SSE broker — the former makes an open Chat tab live-refresh (same event a real turn publishes), the latter drives the toast regardless of which tab is active.

---

## Open Questions / TBD

- Whether the manual Criteria-edit UI needs its own re-order (drag) affordance for `sort_order`, or whether Criteria order stops mattering once editing is direct rather than reparse-driven
