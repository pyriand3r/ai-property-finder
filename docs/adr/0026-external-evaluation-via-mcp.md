---
type: adr
title: "External evaluation: the MCP interface hands out an Evaluation Brief and accepts Grades"
project: property-finder-obs-style
status: accepted
created: 2026-11-24
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. Extends [[0019-mcp-tool-interface]]. This affects [[../../spec/01_mvp/01-04-criteria-and-scoring]] and [[../../spec/01_mvp/01-14-external-evaluation-via-mcp]].

## Context

Evaluation (`EvaluationService.EvaluateListing`) only runs inside the backend. It calls `ai.Provider`, so it needs the backend's own LLM key. An external client on `/mcp` (Claude Code, omp, any other service) cannot do the grading itself. It also cannot ask the server how to grade, or which Criteria exist.

The user wants evaluation to be possible from outside. The external side should get its instructions and Criteria from the MCP server. The internal evaluation stays.

## Decision

Two new MCP tools and one read tool. They call the same service code as the internal path.

- `get_evaluation_brief(listing_id)` returns everything an outside grader needs: the grading instruction, the active Criteria (with IDs), the Listing, the Global and Profile Context, and the user-confirmed fields.
- `submit_evaluation(listing_id, grades[])` stores the Grades. The server maps them by `criterion_id`, validates them, computes the Score itself, and clears `needs_reevaluation`.
- `list_pending_evaluations(profile_id)` lists Listings with `needs_reevaluation = true`. Listings never evaluated also have this flag, so one query covers both.

Rules:

- **One instruction text.** The text of `evaluateSystem` (`internal/ai/ops.go`) is exported and used by the internal call and by the brief. No second copy.
- **One write path.** `EvaluateListing` is split. The part after the LLM call (replace grades, recompute scores, clear the flag, in one transaction) becomes `ApplyGrades`. Internal and external evaluation both end there.
- **Criteria IDs are safe here.** ADR 0006 mapped by position because a model must not invent UUIDs. The brief gives the IDs to the client, and `submit_evaluation` rejects unknown, missing or duplicate IDs. A Grade is `A` to `F`. Reasoning is required.
- **Only active Criteria.** Proposed Criteria are not in the brief and cannot be graded (ADR 0024).
- **No mode switch.** Internal evaluation (button, bulk re-evaluation, `evaluate_listing` Tool) works as before. External evaluation is an additional entry. There is no `evaluation_mode` setting.
- **No marker.** The stored Grades do not say who made them. The UI does not show it.
- **Scope is evaluation only.** Ranking and rejection analysis stay internal.
- **No destructive or criteria-changing MCP tools.** MCP still cannot change active Criteria (ADR 0024) and gets no delete tools. This ADR adds none.

## Alternatives considered

- **`evaluation_mode = internal | external` setting.** Rejected. The user wants the internal path to stay as is. Two entries that share `ApplyGrades` are simpler than a mode.
- **Mark external Grades (`evaluated_by`).** Rejected by the user. It also would need a migration and UI work.
- **Server calls the external service (webhook).** Rejected. The external client already connects over MCP. Pull is simpler than push and needs no outbound config.
- **Mirror the whole REST API as MCP tools** (the first idea for "everything over MCP"). Deferred, not rejected. See `docs/ideas.md`.

## Consequences

- An outside grader can write any Grade. The server checks shape and completeness only, not quality. This is the same trust level as ADR 0019: one operator.
- The Score stays server-computed. A client can never set it.
- Hazard Ratings are not in the brief. A Listing has no link to a Hazard Region yet (see `docs/ideas.md`, "Find the hazard region of a Listing"), and Hazard Ratings are never part of the Score (ADR 0025).
- The grading instruction is code, not a Context File. A user steers grading through the Global and Profile Context, which the brief includes.
- `submit_evaluation` replaces all Grades of the Listing, like the internal path.
