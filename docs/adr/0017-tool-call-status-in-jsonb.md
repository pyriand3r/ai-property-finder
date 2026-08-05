---
type: adr
title: "Confirm-Card status persisted as a field inside chat_message.tool_calls JSONB"
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

ADR 0009 decided the pending proposal lives in `chat_message.tool_calls` JSONB and that "the frontend hides the confirm/reject buttons once either endpoint returns success" — an explicitly frontend-only, in-session concern. Its Consequences section accepted the resulting limitation without spelling it out: nothing server-side records that a proposal was ever confirmed or rejected.

That gap turned out to be user-visible, not just a theoretical loose end: reopening the Chat tab, or reloading the page, remounts `ChatConfirmCard.vue` with its local `status` ref reset to `'pending'` — a Confirm-Card the user already actioned shows the Übernehmen/Verwerfen buttons again, as if nothing happened. Clicking Übernehmen a second time on an already-confirmed `propose_criteria` (action=add) does happen to 409 via the existing name-collision stale-check, so at least it doesn't double-write — but a second click on an already-confirmed `update_context` silently re-appends the same text again (append is not idempotent), and a second Verwerfen on an already-rejected call is a silent no-op that looks like it did something. None of that is acceptable UX once a user actually hits it by navigating around the app normally.

## Decision

**Extend the existing JSONB storage with a `status` field — still no new table, still no new column on any other table.**

`agent.StoredToolCall` (the shape already stored in `chat_message.tool_calls`) gains an optional field:

```go
type StoredToolCall struct {
    ID     string          `json:"id"`
    Name   string          `json:"name"`
    Input  json.RawMessage `json:"input"`
    Status string          `json:"status,omitempty"` // "" | "confirmed" | "rejected"
}
```

On `POST .../confirm`, after the proposal is successfully applied, the confirm endpoint re-reads the message's `tool_calls`, sets `Status = "confirmed"` on the matching entry, and writes the whole array back via a new `ChatRepository.UpdateToolCalls(ctx, messageID, toolCalls)` — a single-column `UPDATE`, same shape as the existing `UpdateTokenCount` method. `POST .../reject` does the same with `Status = "rejected"`.

Both endpoints now also check `Status` before acting: if it's already set, return `409 Conflict` ("bereits bestätigt" / "bereits verworfen") instead of re-applying or re-validating. This closes the non-idempotency gap above as a side effect, not just the reload-resets-to-pending complaint.

The frontend reads this field once, on mount: `ChatConfirmCard.vue` initializes its local `status` ref from a new `status` prop (`ChatMessageList.vue` passes `call.status` straight through) instead of hardcoding `'pending'`. No polling, no extra fetch — the status was already part of the message data the chat view loads anyway.

## Alternatives considered

- **Leave it as pure frontend state (status quo)** — rejected: this is the bug being fixed.
- **Separate `tool_call_status` table (one row per call, or per message)** — rejected for the same reason ADR 0009 rejected a `pending_change` table: duplicates data that already lives in `tool_calls`, adds a join and a cleanup question for no benefit over mutating the JSONB in place.
- **Derive "confirmed" by re-checking whether the proposed state still matches current DB state** (e.g. does a Criterion with this name now exist) — rejected: unreliable for `update_context` (append is not a fingerprint-able operation), and gives no way at all to represent "rejected" (there's no artifact a rejection leaves behind to check against).

## Consequences

- `chat_message.tool_calls` is no longer read-only after insert — the confirm/reject endpoints now mutate it in place. This is a narrow, intentional exception to "tool_calls is what the model requested, immutable" — the mutation is additive (one field) and only ever moves `status` from unset to a terminal value, never rewrites `id`/`name`/`input`.
- `ChatRepository` gains one new method (`UpdateToolCalls`), mirroring `UpdateTokenCount`'s existing shape.
- Confirm/Reject become idempotent from the caller's perspective: a second click anywhere always 409s instead of silently re-applying or duplicating an append.
- No schema migration — `tool_calls` is already `JSONB`, `status` is just a new key inside it that only appears once a call is actioned.
