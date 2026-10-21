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

Accepted. It keeps the principle of an earlier decision in the predecessor project (proposals need user confirmation) but replaces its mechanism. It affects [[../../spec/01_mvp/01-02-context-files#Confirmation]], [[../../spec/01_mvp/01-04-criteria-and-scoring#Rejection Feedback, Reimagined]], and [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]].

## Context

The predecessor project set a principle worth keeping. AI-proposed changes that affect scoring must never apply silently. The user must see a diff and accept it explicitly. It built the principle as one dedicated entity, `profile_proposal`, tied to exactly one trigger (Rejection).

In the chat-driven model, the same need to "propose, don't apply" appears in at least two other places. Neither has anything to do with Rejection:

- The Agent proposes a Context addition after learning something in conversation.
- The Agent proposes new or changed Criteria mid-chat, with no rejection involved.

Three separate proposal mechanisms for the same pattern would triple the surface area for the same UI (diff card, accept/discard).

## Decision

There is one mechanism. Any Agent Tool that would durably change persisted state the user relies on returns a pending-confirmation result. These Tools are `update_context` and `propose_criteria`. The full proposal lives in the tool-call arguments stored in `chat_message.tool_calls` JSONB. The chat turn shows a confirm card with **Übernehmen**/**Verwerfen** (means: "Apply"/"Discard"). Nothing applies until the user acts through a REST endpoint (`POST .../confirm`). There is no dedicated `profile_proposal` table. Implementation: [[0009-proposal-pending-in-tool-calls]].

Rejection-triggered feedback ([[../../spec/01_mvp/01-04-criteria-and-scoring#Rejection Feedback, Reimagined]]) is no longer a special case with its own entity and UI section. It is an ordinary chat turn that happens to end in an `update_context` or `propose_criteria` call. It goes through the same confirm card as everything else.

## Alternatives considered

- **Keep `profile_proposal`, add a second near-identical table for Context proposals** — rejected. It duplicates the same pattern for no benefit. The two tables would drift apart quickly, because they are maintained separately. For example, only one might get a "replace vs. merge" concept.
- **Auto-apply Context/Criteria changes, use `context_revision`/edit history for undo** — rejected. This is the exact failure the principle above was written to prevent. Silent score changes ("why did this listing drop from B to D?") are just as bad whether a Rejection-specific Proposal or an offhand chat remark the Agent over-read caused them.

## Consequences

- One confirm-card UI component serves both Context and Criteria proposals. This means less frontend work than two or three custom flows.
- We lose the "at most one pending Proposal per profile" rule from the predecessor project. In the chat model, a proposal can stay pending from one turn while the next turn is about a different topic. Each confirm card is independent. It does not block unrelated chat turns.
- `evaluate_listing` re-runs automatically once a Criteria proposal is confirmed. The trigger works the same as before ([[../../spec/01_mvp/01-04-criteria-and-scoring]]).
- By default we lose the predecessor project's behaviour "accepting a Proposal resets all Weights to 1". In this model, a Criteria confirm covers exactly what changed (add or edit one Criterion). Existing Weights on untouched Criteria are left alone. No global reset is needed, because there is no full reparse-and-replace step to reset after.
