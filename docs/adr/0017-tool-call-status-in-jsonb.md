---
type: adr
title: "Confirm-Card status is saved as a field inside the chat_message.tool_calls JSONB"
project: property-finder-obs-style
status: accepted
created: 2026-08-05
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — extends [[0009-proposal-pending-in-tool-calls]]. Affects [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/01_mvp/01-02-context-files#Confirmation]], [[../../spec/01_mvp/01-09-chat-reliability-fixes]].

## Context

ADR 0009 decided that the pending proposal lives in the `chat_message.tool_calls` JSONB. It also said "the frontend hides the confirm/reject buttons once either endpoint returns success". That is a frontend-only concern, limited to the current session. Its Consequences section accepted the resulting limit without spelling it out: nothing on the server records that a proposal was ever confirmed or rejected.

This gap is visible to the user. It is not just a theoretical loose end. When the user reopens the Chat tab or reloads the page, `ChatConfirmCard.vue` mounts again, and its local `status` ref resets to `'pending'`. A Confirm-Card the user already actioned shows the Übernehmen/Verwerfen buttons again (means: "Apply"/"Discard"), as if nothing happened. What happens on a second click:

- Clicking Übernehmen a second time on an already-confirmed `propose_criteria` (action=add) returns a 409. This comes from the existing name-collision stale-check. So at least it does not write twice.
- A second click on an already-confirmed `update_context` quietly adds the same text again. Append is not idempotent.
- A second Verwerfen on an already-rejected call does nothing, but looks like it did something.

None of that is acceptable UX once a user hits it by moving around the app in the normal way.

## Decision

**Extend the existing JSONB storage with a `status` field — still no new table, still no new column on any other table.**

`agent.StoredToolCall` (the shape already stored in `chat_message.tool_calls`) gets an optional field:

```go
type StoredToolCall struct {
    ID     string          `json:"id"`
    Name   string          `json:"name"`
    Input  json.RawMessage `json:"input"`
    Status string          `json:"status,omitempty"` // "" | "confirmed" | "rejected"
}
```

On `POST .../confirm`, the proposal is applied first. Then the confirm endpoint re-reads the `tool_calls` of the message and sets `Status = "confirmed"` on the matching entry. It writes the whole array back with a new `ChatRepository.UpdateToolCalls(ctx, messageID, toolCalls)`. This is a single-column `UPDATE`, the same shape as the existing `UpdateTokenCount` method. `POST .../reject` does the same with `Status = "rejected"`.

Both endpoints now also check `Status` before they act. If it is already set, they return `409 Conflict` ("bereits bestätigt" / "bereits verworfen", means: "already confirmed" / "already rejected"). They do not apply or validate again. As a side effect, this also closes the non-idempotency gap above, not just the reload-resets-to-pending complaint.

The frontend reads this field once, on mount. `ChatConfirmCard.vue` sets its local `status` ref from a new `status` prop instead of hardcoding `'pending'`. `ChatMessageList.vue` passes `call.status` straight through. There is no polling and no extra fetch. The status is already part of the message data that the chat view loads anyway.

## Alternatives considered

- **Leave it as pure frontend state (status quo)** — rejected: this is the bug being fixed.
- **Separate `tool_call_status` table (one row per call, or per message)** — rejected for the same reason ADR 0009 rejected a `pending_change` table. It copies data that already lives in `tool_calls`. It adds a join and a cleanup question, with no benefit over changing the JSONB in place.
- **Work out "confirmed" by checking whether the proposed state still matches the current DB state** (for example, does a Criterion with this name now exist) — rejected: it is unreliable for `update_context`, because append cannot be fingerprinted. It also cannot show "rejected" at all, because a rejection leaves nothing behind to check.

## Consequences

- `chat_message.tool_calls` is no longer read-only after insert. The confirm and reject endpoints now change it in place. This is a narrow, deliberate exception to "tool_calls is what the model requested, immutable". The change only adds one field. It only ever moves `status` from unset to a final value. It never rewrites `id`, `name` or `input`.
- `ChatRepository` gets one new method (`UpdateToolCalls`), with the same shape as the existing `UpdateTokenCount`.
- Confirm and Reject become idempotent from the caller's view. A second click anywhere always returns 409. It no longer quietly applies again or adds a duplicate append.
- No schema migration. `tool_calls` is already `JSONB`. `status` is just a new key inside it, and it only appears once a call is actioned.
