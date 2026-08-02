---
type: adr
title: "One unified confirm-before-apply mechanism replaces the Proposal entity"
project: property-finder-obs-style
status: accepted
created: 2026-07-28
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — carries forward the principle of [[../../../dev-property-finder/docs/adr/0001-proposal-requires-user-confirmation|dev-property-finder ADR 0001]], replaces its mechanism. Affects [[../../spec/01_mvp/01-02-context-files#Confirmation]], [[../../spec/01_mvp/01-04-criteria-and-scoring#Rejection Feedback, Reimagined]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]].

## Context

dev-property-finder ADR 0001 established a real principle worth keeping: AI-proposed changes that affect scoring must never apply silently — the user has to see a diff and explicitly accept it. It implemented that principle as one dedicated entity, `profile_proposal`, wired to exactly one trigger (Rejection). In the chat-driven model, the same need for "propose, don't apply" shows up in at least two other places that have nothing to do with Rejection: the Agent proposing a Context addition after learning something in conversation, and the Agent proposing new/changed Criteria mid-chat, unprompted by any rejection at all. Three separate proposal mechanisms for the same underlying pattern would triple the surface area for the same UI (diff card, accept/discard).

## Decision

There is one mechanism: any Agent Tool that would durably change persisted state the user relies on — `update_context`, `propose_criteria` — returns a pending-confirmation result and the full proposal lives in the tool-call arguments stored in `chat_message.tool_calls` JSONB. The chat turn renders a confirm card with **Übernehmen**/**Verwerfen**. Nothing applies until the user acts via a REST endpoint (`POST .../confirm`). No dedicated `profile_proposal` table. Implementation: [[0009-proposal-pending-in-tool-calls]].

Rejection-triggered feedback ([[../../spec/01_mvp/01-04-criteria-and-scoring#Rejection Feedback, Reimagined]]) is no longer a special case with its own entity and UI section — it's an ordinary chat turn that happens to end in an `update_context` or `propose_criteria` call, going through the same confirm card as anything else.

## Alternatives considered

- **Keep `profile_proposal`, add a second near-identical table for Context proposals** — rejected: duplicates the same pattern for no benefit; the two would immediately drift (e.g. only one gets a "replace vs. merge" concept) since they're maintained separately.
- **Auto-apply Context/Criteria changes, rely on `context_revision`/edit history for undo** — rejected: this is the exact failure mode ADR 0001 was written to prevent — silent score changes ("why did this listing drop from B to D?") are just as bad whether triggered by a Rejection-specific Proposal or an offhand chat remark the Agent over-interpreted.

## Consequences

- One confirm-card UI component serves both Context and Criteria proposals, reducing frontend surface versus two/three bespoke flows
- Losing the "at most one pending Proposal per profile" constraint dev-property-finder had — the chat model can have a proposal pending from one turn and another topic entirely in the next; each confirm card is independent and doesn't block unrelated chat turns
- `evaluate_listing` re-runs automatically once a Criteria proposal is confirmed, same trigger semantics as before ([[../../spec/01_mvp/01-04-criteria-and-scoring]])
- Losing dev-property-finder's "accepting a Proposal resets all Weights to 1" behaviour by default — a Criteria confirm in this model is scoped to exactly what changed (add/edit one Criterion), so existing Weights on untouched Criteria are simply left alone; no global reset needed since there's no full reparse-and-replace step to reset after
