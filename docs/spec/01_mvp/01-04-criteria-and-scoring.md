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

This spec reuses the data model and formula of the predecessor project word for word. That means: Objective and Subjective Criteria, an integer Weight from 1 to 5, and an A–F Grade per Criterion with reasoning. The formula is `score = sum(grade_value * weight) / sum(weight)`, mapped A=100…F=0. This spec covers only what is new. It covers how Criteria are created and changed. It also covers direct manual editing.

---

## Criteria Origin

Criteria enter a profile in three ways. All three end up in the same `criterion` table:

1. **Initial conversational parse**: this happens the first time a Source is ingested into an empty profile. The Agent calls `propose_criteria`. It bases the proposal on the conversation so far (what the user said they are looking for) and anything already in the Profile Context. It shows the proposal as a Confirm-Card, the same pattern as Context proposals ([[01-02-context-files#Confirmation]]).
2. **Conversational addition**: the user says "auch nach Nähe zum Wasser bewerten" (means: "also rate by closeness to water") during a chat. The Agent calls `propose_criteria` again. This time it is a diff against the existing Criteria. It adds or changes one Criterion. It does not replace them all.
3. **Direct manual edit**: the predecessor project explicitly left this out of its MVP. Here the user can add, edit, or remove a Criterion directly in a Criteria list UI on the profile. The user can set name, description, Objective/Subjective toggle, and Weight slider. The Agent is not involved at all.

**Proposed and active** ([[../../docs/adr/0024-criterion-proposal-status]]): a Criterion created by the Agent (chat `add`) or by an MCP client is stored as `proposed`. A proposed Criterion shows a "Vorgeschlagen" badge and a "Freischalten" button in the Criteria tab. It can be edited and deleted. `evaluate_listing`, the Score and the ranking ignore it until it is approved (`POST /api/criteria/{id}/approve`, or the Confirm-Card in chat). A Criterion made in the form is `active` at once. MCP cannot change an active Criterion. Names are unique per profile (case-insensitive, proposed and active together). Changing or deleting a *proposed* Criterion does not flag Listings. Approving one does.

Any Criteria change sets `listing.needs_reevaluation = true` on all Listings in the profile. This covers adding or removing a Criterion. It also covers edits to name, description, or the Objective/Subjective flag. It applies whether the change is conversational or manual. **Weight-only changes are the exception.** They recompute every Score in pure SQL, with no flag and no LLM call. See Weight below. The user starts re-evaluation explicitly. The user can do it per listing from the detail view, or in bulk from the banner in the list view. There is no automatic re-evaluation. See [[../../docs/adr/0015-manual-bulk-reevaluation]].

---

## Weight

Unchanged: an integer from 1 to 5, default 1, set with a slider. A Weight-only change recomputes the Score in pure SQL, with no LLM call.

---

## Grade & Reasoning

Unchanged: `evaluate_listing` returns one Grade and its reasoning for each Criterion. The backend maps them by position, not by an echoed ID. Every Evaluation fully replaces the old one. The Listing detail view shows the result in the same grid as the predecessor project.

---

## Rejection Feedback, Reimagined

The user can reject a Listing through the detail form or in chat. Then the backend at once starts an asynchronous LLM call. The call analyses the rejection comment. It may produce `propose_criteria` and/or `update_context` proposals. The results appear as Confirm-Cards in the profile's Chat. A toast tells the user: „Rejection verarbeitet — Vorschläge im Chat verfügbar." (means: "Rejection processed — suggestions available in chat.")

The analysis call is a single-shot LLM request, with no Agent loop and no tools. It follows the same pattern as `ParseSearchProfile` in the predecessor project. It also has the same shape this project already uses for `EvaluateListing`/`GenerateRanking` (`internal/ai`, `RejectionAnalysisService`, Layer 8.1). It does not start re-evaluation. It only creates proposals. `needs_reevaluation` is set on all listings only after the user confirms a Criteria proposal (through the manual trigger from ADR 0015).

The call runs only when a Listing changes *to* `rejected`. It does not run on later saves of a Listing that is already rejected. It also runs only when a `rejection_comment` exists. It runs in a detached goroutine (the ADR 0008 fire-and-forget shape), so the PATCH response is not delayed. Often the model finds nothing worth proposing. Then nothing is written: no empty chat message and no toast. When the model does find something, the proposals are written as one synthetic `chat_message`. It has `role=assistant`. Its `tool_calls` have exactly the same shape that `propose_criteria`/`update_context` calls would have in a live Agent turn. The message is appended to the profile's active session. The existing Confirm-Card UI and the `/tool-calls/{idx}/confirm|reject` endpoints (ADR 0009/0017) handle it with zero changes. For that code, it is just another tool call. The SSE broker publishes `chat.message` and `listing.rejection_analyzed`. `chat.message` makes an open Chat tab refresh live (the same event a real turn publishes). `listing.rejection_analyzed` drives the toast, whichever tab is active.

---

## Open Questions / TBD

- Does the manual Criteria-edit UI need its own drag control to re-order `sort_order`? Or does Criteria order stop mattering once editing is direct and no longer driven by re-parsing?
