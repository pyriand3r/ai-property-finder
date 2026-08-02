---
type: adr
title: "Pending proposals live in chat_message.tool_calls JSONB, confirmed via REST endpoints"
project: property-finder-obs-style
status: accepted
created: 2026-08-01
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — implements the confirm-before-apply principle of [[0004-unified-confirm-before-apply]] with a concrete data flow. Affects [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/01_mvp/01-02-context-files]].

## Context

ADR 0004 established that `update_context` and `propose_criteria` must never silently apply changes — the user sees a diff and explicitly accepts or rejects. The implementation was left open: where does the pending proposal live between the Agent suggesting it and the user confirming it?

The Agent loop is a multi-turn tool-use cycle inside a single HTTP request (POST → SSE stream). The loop cannot pause and wait for the user to click "Übernehmen" — by the time the diff card renders in the frontend, the HTTP request has already closed and the loop has moved on. So the pending state must live somewhere durable that outlasts the loop.

Several places were considered: a dedicated `pending_change` table, a `status` column on `context_revision`, or holding the diff purely in the tool-call arguments that Claude already formatted correctly.

## Decision

**The proposal lives in `chat_message.tool_calls` JSONB — no new tables, no staging layer.**

When the Agent calls `propose_criteria` or `update_context`, the tool implementation:

1. **Does NOT write** to `criterion` or `context.content_md`
2. **Validates** the proposed change (e.g. criterion name is not empty, weight is 1–5)
3. **Returns** a result to the Agent loop:
   ```json
   {
     "status": "pending_confirmation",
     "summary": "Vorgeschlagen: Kriterium 'Nähe zum Wasser' (Gewicht 3). ⚠️ Noch nicht aktiv – Bestätigung ausstehend."
   }
   ```

The `status` field tells the Agent: „Do not use this for `evaluate_listing` in the current turn. Tell the user they need to confirm.“ The Agent's chat reply will include both the `summary` text and Claude's own explanation. The frontend renders a diff card next to the message because the message's `tool_calls` array contains the full proposal.

### Tool-Call Arguments as Source of Truth

Claude already formats the proposal into the tool call arguments as valid JSON:

```json
// propose_criteria arguments
{
  "action": "add",
  "name": "Nähe zum Wasser",
  "description": "Entfernung zum nächsten See oder Meer in Gehweite",
  "is_objective": false,
  "weight": 3
}

// update_context arguments
{
  "scope": "profile",
  "operation": "append",
  "content": "Bei Booli-Inseraten immer dem 'Läs mer hos mäklaren'-Link folgen.\n\nGrund: Die Beschreibung auf der Maklerseite ist meist ausführlicher als das Hemnet/ Booli-Inserat."
}
```

These arguments are stored automatically in `chat_message.tool_calls` (JSONB) as part of persisting the turn. No extra processing, no extraction step — they're already structured, already validated by Claude's tool-use schema, and already versioned by being part of a specific chat message.

### Confirm / Reject Endpoints

Two REST endpoints, thin wrappers:

| Endpoint | Behaviour |
|---|---|
| `POST /api/chat/messages/{msg_id}/tool-calls/{call_idx}/confirm` | Reads `chat_message.tool_calls[call_idx]`, validates against current state (no stale applies), writes to target table (`criterion` or `context`), returns `200` + updated state |
| `POST /api/chat/messages/{msg_id}/tool-calls/{call_idx}/reject` | Returns `200` — the rejection is implicit by not confirming; this endpoint exists for the frontend to cleanly dismiss the card |

The frontend hides the confirm/reject buttons once either endpoint returns success. The diff card remains visible in chat history but the buttons turn into a label ("Übernommen" / "Verworfen").

### Stale Detection

Between proposal and confirm, the user might have manually changed the affected entity. On confirm, the endpoint checks:

- **For `propose_criteria`** (`action = "add"`): a criterion with the same name must not already exist. `409 Conflict` → frontend shows "Vorschlag veraltet — Kriterium existiert bereits."
- **For `propose_criteria`** (`action = "edit"`): the referenced criterion must still exist and must not have been manually changed since the proposal. If changed → `409 Conflict` → "Vorschlag veraltet."
- **For `update_context`**: always applies (appending/merging text), because the current `content_md` is compared as a base — the proposal is a plaintext append, not a structured patch. If the base text changed, `409 Conflict` → frontend shows the current vs. proposed diff so the user can decide.
- **For `reject`**: no validation needed — accepting nothing is always safe.

No automatic merging, no diff3. Simple and predictable.

### Re-Evaluation After Confirm

Once a Criteria proposal is confirmed (new or changed Criterion), the backend triggers full re-evaluation of all Listings in the profile — exactly as specified in [[../../spec/01_mvp/01-04-criteria-and-scoring]] and ADR 0004's consequences. The difference: now the trigger is the confirm endpoint, not an implicit agent-turn side effect.

## Alternatives considered

- **Dedicated `pending_change` table** — rejected: duplicates the proposal data that already exists in `tool_calls` JSONB. Adds a schema migration, a repository, and a cleanup problem (when to delete stale pending rows?).
- **`status` column on `context_revision`** — rejected: `context_revision` is an append-only audit log with snapshot-before-change semantics. Making it also a staging table conflates two concerns (history vs. proposals) and doesn't cover `propose_criteria` at all.
- **Hold pending state purely in frontend memory** (no server-side storage, the tool call is extracted client-side from the SSE stream and the confirm POST sends the full proposal back) — rejected: fragile on reload, and the server can't validate what's being applied without storing the proposal somewhere server-side. `tool_calls` JSONB is that server-side store without any extra infrastructure.

## Consequences

- `context_revision` stays a pure audit log — no `status` column, no pending rows. A revision is created only when `content_md` is actually changed (on confirm), not on proposal.
- The `update_context` and `propose_criteria` tool implementations are read-only from the perspective of their target tables — they validate and return, they don't write.
- Two lightweight REST endpoints, ~30 lines each: validate, write, return.
- The frontend confirm-card component receives its data from the SSE stream (tool-call arguments already in the event), not from a separate pending-proposals API — the chat message *is* the API response.
- Stale detection is simple (check if target still exists / hasn't changed) but not lossless (the user can't "merge" a stale proposal with manual edits automatically). Accepted for MVP: in a single-user app, the user sees the conflict, understands it, and asks the Agent again if needed.
