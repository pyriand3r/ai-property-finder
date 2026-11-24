---
type: spec
created: 2026-11-24
title: External Evaluation via MCP
project: property-finder-obs-style
spec: external-evaluation-via-mcp
phase: 1
order: 14
category: criteria
status: ready
updated: 2026-11-24
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - mcp
  - evaluation
---

## Overview

An external client on `/mcp` can grade a Listing itself. It asks the server how to grade and which Criteria exist, grades, and sends the Grades back. The server does not need an LLM key for this. The internal evaluation stays and is unchanged. Decision: [[../../adr/0026-external-evaluation-via-mcp]].

Out of scope: ranking, rejection analysis, new tools for profiles/listings/notes/images, deleting, changing active Criteria, a marker for who graded.

## Layer 1: Shared code

- Export the grading instruction from `internal/ai/ops.go` (`EvaluationInstruction`). `runEvaluate` uses it. No copy.
- Split `EvaluationService.EvaluateListing`. New `ApplyGrades(ctx, listingID, grades []domain.CriterionScore)` runs the transaction that is there today (`ReplaceForListing`, `RecomputeScores`, `ClearNeedsReevaluation`). `EvaluateListing` calls it.
- New `EvaluationService.Brief(ctx, listingID)` and `SubmitGrades(ctx, listingID, []SubmittedGrade)`.

## Layer 2: MCP tools

All three live in `internal/mcpserver`, next to the existing tools. They call the services directly (like `list_profiles`), not the agent Registry.

**`get_evaluation_brief`** `{listing_id}` returns:

|Field|Content|
|---|---|
|`instruction`|`EvaluationInstruction`, adapted to the by-ID format (see below)|
|`criteria`|Active Criteria: `id`, `name`, `description`, `weight`, `is_subjective`, in a stable order|
|`listing`|The Listing as the internal prompt sees it (`renderListing`), plus `document_md`|
|`global_context`, `profile_context`|Markdown of both Context Files|
|`user_confirmed_fields`|As in ADR 0016|
|`response_format`|`[{"criterion_id": "<uuid>", "grade": "A".."F", "reasoning": "..."}]`|

Error: the profile has no active Criteria (same `ValidationError` as `EvaluateListing`).

The internal prompt says "by position". The brief must say "by `criterion_id`". So the instruction is built from a shared base plus a format part. Do not ship a text that contradicts the response format.

**`submit_evaluation`** `{listing_id, grades: [{criterion_id, grade, reasoning}]}`:

- Every active Criterion must have exactly one Grade. Unknown, duplicate or missing `criterion_id` is a `ValidationError` naming the ID.
- `grade` is normalized with the existing `normalizeGrade` (exported). Anything but A to F is rejected.
- `reasoning` must not be empty after trim.
- On success: `ApplyGrades`, then returns the updated Listing with `overall_score` and the stored Grades.
- Nothing is written when validation fails.

**`list_pending_evaluations`** `{profile_id}` returns Listings with `needs_reevaluation = true`: `id`, `title`, `status`, `overall_score` (nullable). Uses `ListByProfile(..., &stale)` as `ReevaluateAll` does.

## Layer 3: Tests

Stdlib only, after `propose_criteria_test.go` (needs `TEST_DATABASE_URL`):

- Brief lists only active Criteria, not proposed ones.
- Submit with a missing, a duplicate, an unknown ID, a bad grade, an empty reasoning: error, grades unchanged.
- Valid submit: Grades stored, `overall_score` equals the server computation, `needs_reevaluation` false.
- `list_pending_evaluations` lists a stale Listing and drops it after submit.
- Unit test: the internal `runEvaluate` still produces the same prompt after the refactor.

## Layer 4: Docs

- `AGENTS.md`: Tool/MCP list, ADR index, build state.
- `docs/CONTEXT.md`: **Evaluation Brief**.
- `docs/adr/README.md`: ADR 0026 row. Mark 0019 as extended.
- `docs/spec/00_infrastructure_setup/00-03-llm-agent-integration.md`: MCP section lists the three tools.
- MCP client notes in `docs/operations.md` if it documents the tool list.

## Acceptance

With only an MCP client and no `CLAUDE_API_KEY`: `list_pending_evaluations` → `get_evaluation_brief` → `submit_evaluation` stores Grades, and the Score and the Listing view in the UI show them.
