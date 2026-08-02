---
type: adr
title: "Re-evaluation is manual, not automatic; user triggers per-listing or bulk from the UI"
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

The spec had two conflicting statements about what triggers re-evaluation:

- `01-04`: „Any Criteria change — conversational or manual — triggers full re-evaluation of every Listing in the profile“
- `01-07`: „Editing any structured field that participates in Criteria evaluation triggers re-evaluation of that Listing“
- ADR 0004 Consequences: „`evaluate_listing` re-runs automatically once a Criteria proposal is confirmed“

Automatic re-evaluation is dangerous at scale: a single Weight tweak triggers N LLM calls (one per listing) with no user confirmation, no cost warning, and no way to batch changes. The user might edit three Criteria weights in sequence, triggering 3 × N = potentially 60+ paid API calls before they realise what's happening.

At the same time, the user needs to test changes: edit one Criterion, re-evaluate *one* listing to see if the scores make sense, then apply to all. And they need to see at a glance which listings are stale after Criteria changes.

## Decision

**Re-evaluation is never automatic. The user triggers it explicitly from the UI.**

### Staleness Indicator

`listing.needs_reevaluation BOOLEAN DEFAULT false`. Set to `true` when:

- Any Criterion is added, edited, or removed (manual or confirmed via `propose_criteria`)
- Any structured field of the listing is manually edited ([[01-07-manual-review-and-edit]])

Set to `false` when `evaluate_listing` completes successfully for that listing (via Agent Tool during chat, or via the manual buttons below).

### UI Affordances

| Location | Affordance | Scope |
|---|---|---|
| Listing detail view | Button „Neu bewerten“ | This listing only (1 LLM call) |
| Listing list header | Banner „Bewertung veraltet — Kriterien wurden geändert. N von M Inseraten neu bewerten?“ + Button „Alle neu bewerten“ | All listings with `needs_reevaluation = true` |
| Listing list rows | ⚠️ Icon next to score | Indicates `needs_reevaluation = true` for this row |

The banner appears only when at least one listing has `needs_reevaluation = true`. It counts affected listings. The button triggers a sequential bulk run.

### Bulk Execution

1. User clicks „Alle neu bewerten“
2. Backend starts a goroutine (not a `task` row — same fire-and-forget pattern as Agent turns per [[0008-agent-turn-fire-and-forget]])
3. Iterates `listings WHERE needs_reevaluation = true` ordered by `created_at`
4. Calls `evaluate_listing` sequentially (same service function as the Agent Tool uses)
5. SSE stream delivers progress: `{listing_id, listing_title, status: "running" | "done" | "error"}`
6. One listing failing does not stop the rest — errors are collected and shown in a summary at the end
7. On completion: banner disappears, all ⚠️ icons cleared (except errored rows)

No `task` table, no recovery, no parallel execution. Sequential is correct: respects Claude API rate limits automatically, and the `evaluate_listing` LLM call is fast (~2–5 seconds per listing) — 20 listings take ~1–2 minutes, well within user patience for a manual action.

### Agent Still Triggers Per-Listing

During chat ingestion, the Agent calls `evaluate_listing` as a Tool on the freshly saved listing. This is automatic *within the chat turn* — the user just told the Agent to ingest, and evaluation is part of that workflow. The `needs_reevaluation` flag is set to `false` by this call (it's a normal `evaluate_listing` invocation). No banner appears for this single listing because its flag was just cleared.

### Why Not Automatic

- **Cost surprise:** 3 Weight tweaks × 20 listings = 60 API calls. User didn't click anything that said "this costs money."
- **Batch intent:** User often makes several Criteria changes together before wanting scores to reflect them.
- **Test workflow:** User changes one Weight, re-evaluates one listing to check, then applies to all. Automatic re-eval skips the test step.
- **KISS:** `needs_reevaluation = true` + a button is ~5 lines of backend logic. A queued background job with deduplication and partial-failure semantics is ~100+ lines.

## Alternatives considered

- **Automatic re-evaluation on every change** (as originally spec'd) — rejected: cost surprise and no batch/test workflow.
- **Debounced automatic re-evaluation** (wait 30 seconds after last change, then run) — rejected: still runs without explicit user intent. The user might walk away, come back, and have no idea why their API bill is higher.
- **Chat-only re-evaluation** (Agent must be asked in chat to re-evaluate) — rejected: too slow for bulk and too indirect for single-listing checks. The UI buttons are faster than typing in chat.

## Consequences

- `listing` gains one column: `needs_reevaluation BOOLEAN DEFAULT false`
- Criteria-change handler sets `needs_reevaluation = true` on all listings in the profile — one `UPDATE` statement, no loop
- Manual field-edit handler sets `needs_reevaluation = true` on the edited listing
- `evaluate_listing` sets `needs_reevaluation = false` on success — whether called by Agent Tool or bulk button
- Bulk goroutine: ~20 lines (iterate, call service, emit SSE events, collect errors)
- Frontend banner: ~15 lines (watch SSE events, count affected listings, show/hide)
- The `task` table still exists for Phase 3 automated recurring search — re-evaluation does not use it
