---
type: adr
title: "Re-evaluation is manual, not automatic; the user starts it per listing or in bulk from the UI"
project: property-finder-obs-style
status: accepted
created: 2026-08-02
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — affects [[../../spec/01_mvp/01-04-criteria-and-scoring]], [[../../spec/01_mvp/01-05-ranking-and-list-view]], [[../../spec/01_mvp/01-07-manual-review-and-edit]].

## Context

The spec had two statements that disagreed about what starts re-evaluation. A third came from an ADR:

- `01-04`: „Any Criteria change — conversational or manual — triggers full re-evaluation of every Listing in the profile“
- `01-07`: „Editing any structured field that participates in Criteria evaluation triggers re-evaluation of that Listing“
- ADR 0004 Consequences: „`evaluate_listing` re-runs automatically once a Criteria proposal is confirmed“

Automatic re-evaluation is dangerous at scale. One small Weight change triggers N LLM calls (one per listing). There is no user confirmation, no cost warning and no way to batch changes. The user might edit three Criteria weights in a row. That triggers 3 × N calls, which can be 60+ paid API calls, before the user notices.

The user also needs to test changes. They edit one Criterion, re-evaluate *one* listing to see if the scores make sense, then apply the change to all. They also need to see at a glance which listings are stale after Criteria changes.

## Decision

**Re-evaluation is never automatic. The user starts it explicitly from the UI.**

### Staleness Indicator

`listing.needs_reevaluation BOOLEAN DEFAULT false`. It is set to `true` when:

- Any Criterion is added, edited or removed (manually or confirmed through `propose_criteria`)
- Any structured field of the listing is edited by hand ([[01-07-manual-review-and-edit]])

It is set to `false` when `evaluate_listing` finishes successfully for that listing. This can happen through the Agent Tool during chat, or through the manual buttons below.

### UI Affordances

| Location | Affordance | Scope |
|---|---|---|
| Listing detail view | Button „Neu bewerten“ (means: "Re-evaluate") | This listing only (1 LLM call) |
| Listing list header | Banner „Bewertung veraltet — Kriterien wurden geändert. N von M Inseraten neu bewerten?“ (means: "Evaluation out of date — Criteria changed. Re-evaluate N of M listings?") + Button „Alle neu bewerten“ (means: "Re-evaluate all") | All listings with `needs_reevaluation = true` |
| Listing list rows | ⚠️ Icon next to score | Shows that `needs_reevaluation = true` for this row |

The banner appears only when at least one listing has `needs_reevaluation = true`. It counts the affected listings. The button starts a sequential bulk run.

### Bulk Execution

1. The user clicks „Alle neu bewerten“.
2. The backend starts a goroutine. It does not use a `task` row. This is the same fire-and-forget pattern as Agent turns in [[0008-agent-turn-fire-and-forget]].
3. It goes through `listings WHERE needs_reevaluation = true`, ordered by `created_at`.
4. It calls `evaluate_listing` one listing after another (the same service function the Agent Tool uses).
5. The SSE stream sends progress: `{listing_id, listing_title, status: "running" | "done" | "error"}`
6. One failing listing does not stop the rest. Errors are collected and shown in a summary at the end.
7. When the run ends, the banner disappears and all ⚠️ icons are cleared (except on rows with errors).

There is no `task` table, no recovery and no parallel execution. Running one after another is correct. It respects the Claude API rate limits automatically. The `evaluate_listing` LLM call is fast (~2–5 seconds per listing). 20 listings take about 1–2 minutes. That is fine for a manual action.

### Agent Still Triggers Per-Listing

During chat ingestion, the Agent calls `evaluate_listing` as a Tool on the listing it just saved. This is automatic *inside the chat turn*. The user just told the Agent to ingest, and evaluation is part of that workflow. This call sets the `needs_reevaluation` flag to `false` (it is a normal `evaluate_listing` call). No banner appears for this one listing, because its flag was just cleared.

### Why Not Automatic

- **Cost surprise:** 3 Weight changes × 20 listings = 60 API calls. The user did not click anything that said "this costs money."
- **Batch intent:** The user often makes several Criteria changes together before they want the scores to show them.
- **Test workflow:** The user changes one Weight, re-evaluates one listing to check, then applies it to all. Automatic re-evaluation skips the test step.
- **KISS:** `needs_reevaluation = true` plus a button is about 5 lines of backend logic. A queued background job with deduplication and partial-failure rules is about 100+ lines.

## Alternatives considered

- **Automatic re-evaluation on every change** (as first written in the spec) — rejected: cost surprise, and no batch or test workflow.
- **Debounced automatic re-evaluation** (wait 30 seconds after the last change, then run) — rejected: it still runs without clear user intent. The user might walk away, come back, and not know why the API bill is higher.
- **Chat-only re-evaluation** (the user must ask the Agent in chat to re-evaluate) — rejected: too slow for bulk and too indirect for single-listing checks. The UI buttons are faster than typing in chat.

## Consequences

- `listing` gets one new column: `needs_reevaluation BOOLEAN DEFAULT false`.
- The Criteria-change handler sets `needs_reevaluation = true` on all listings in the profile. This is one `UPDATE` statement, no loop.
- The manual field-edit handler sets `needs_reevaluation = true` on the edited listing.
- `evaluate_listing` sets `needs_reevaluation = false` on success, whether the Agent Tool or the bulk button called it.
- The bulk goroutine is about 20 lines (loop, call the service, send SSE events, collect errors).
- The frontend banner is about 15 lines (watch SSE events, count affected listings, show or hide).
- The `task` table still exists for Phase 3 automated recurring search. Re-evaluation does not use it.
