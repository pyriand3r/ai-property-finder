---
type: adr
title: "Criteria have a proposed/active status; agents and MCP clients only propose"
project: property-finder-obs-style
status: accepted
created: 2026-11-23
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. Partly replaces [[0017-tool-call-status-in-jsonb]] and [[0019-mcp-tool-interface]] (for criteria only, see Consequences). Affects [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/01_mvp/01-04-criteria-and-scoring]] and [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]]. Decided in issue #29.

## Context

Criteria are the basis of every Score. Before this ADR, every path wrote an *active* criterion at once:

- Chat: `propose_criteria` staged a proposal in `chat_message.tool_calls`. The Confirm-Card then created the criterion (ADR 0009/0017).
- MCP: `propose_criteria` wrote directly, for add **and** edit (ADR 0019). An external client could change active criteria with no human check.

A later background agent (issue #18) also needs a place to collect suggestions without changing the ranking.

## Decision

1. `criterion.status` is `TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('proposed', 'active'))` (migration `000004_criterion_status`). `DEFAULT 'active'` makes every existing criterion active in the same statement. TEXT + CHECK, not BOOLEAN, so later states (for example `rejected`) can be added.
2. Only `active` criteria are used by `evaluate_listing`, the score formula (`RecomputeScores`) and therefore the ranking. A `proposed` criterion is visible, editable and deletable, but ignored. Adding, editing or deleting a proposed criterion does not flag listings for re-evaluation. Approving one does, like adding an active criterion.
3. **MCP never changes active criteria.** `propose_criteria` over MCP can only add a criterion (status `proposed`) or edit one that is still `proposed`. `action=edit` with the id of an active criterion is an error and writes nothing. Changing or deleting an active criterion stays with the chat Confirm-Card and the plain form.
4. **Names are unique per profile**, case-insensitive, across proposed and active criteria. `CriterionService` enforces it for create, propose and rename (409).
5. **Approval**: `POST /api/criteria/{id}/approve` (`CriterionService.Approve`) sets `status = 'active'`. The Criteria tab shows a "Vorgeschlagen" badge and a "Freischalten" button.
6. **Chat**: `propose_criteria` with `action=add` stores a `proposed` row when the tool runs. The Confirm-Card ("Freischalten") approves that row instead of creating one. "Verwerfen" deletes it. The card finds the row by criterion name (unique, see 4). `action=edit` is unchanged: the proposal stays in `tool_calls` until confirmed, then updates the criterion.
7. The plain form and `POST /api/profiles/{id}/criteria` still create `active` criteria at once. The form is the user's own, authoritative write (ADR 0001).

## Consequences

- ADR 0017 stays valid for the Confirm-Card status and for `update_context` and criterion edits. It no longer describes how a *new* criterion is created: the row exists before the card is clicked. ADR 0009 ("nothing is written before confirmation") therefore no longer holds for new criteria.
- ADR 0019 stays valid for the MCP transport, auth and `update_context`. Its "writes immediately" rule no longer applies to `propose_criteria`.
- Proposals from the rejection analysis (spec 01-04) are still written only to `chat_message.tool_calls`, with no row. For them the Confirm-Card has nothing to approve, so Confirm creates the criterion as `active`, as before.
- A user can delete a proposed row in the Criteria tab before clicking the card. Confirm then also creates an active criterion (same case as above). This is accepted: the user clicked "Freischalten".
- A proposed criterion that is never approved stays forever. There is no expiry and no bulk approval (out of scope).
- No `approved_at` column. `updated_at` is set on approval.

## Alternatives considered

- **Let MCP edit active criteria as a new proposal** — rejected: needs a second row per criterion or a pending-change column, and a name collision rule for it. Not worth it for the MVP.
- **BOOLEAN `is_active`** — rejected: cannot grow into more states.
- **Keep chat proposals only in `tool_calls`** — rejected: the issue asks for one proposal list in the Criteria tab, for chat and MCP alike.
