---
type: adr
title: "Pending proposals are stored in chat_message.tool_calls JSONB and confirmed through REST endpoints"
project: property-finder-obs-style
status: accepted
created: 2026-08-01
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. This gives the confirm-before-apply principle of [[0004-unified-confirm-before-apply]] a concrete data flow. It affects [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], and [[../../spec/01_mvp/01-02-context-files]].

## Context

ADR 0004 says that `update_context` and `propose_criteria` must never apply changes silently. The user sees a diff and accepts or rejects it. ADR 0004 did not say how to build this. Where does the pending proposal live between the Agent suggesting it and the user confirming it?

The Agent loop is a multi-turn tool-use cycle inside a single HTTP request (POST → SSE stream). The loop cannot pause and wait for the user to click "Übernehmen" (means: "accept"). When the diff card shows in the frontend, the HTTP request has already closed and the loop has moved on. So the pending state must live in a durable place that outlasts the loop.

We looked at several places: a separate `pending_change` table, a `status` column on `context_revision`, or keeping the diff only in the tool-call arguments that Claude already formatted correctly.

## Decision

**The proposal lives in `chat_message.tool_calls` JSONB. There are no new tables and no staging layer.**

When the Agent calls `propose_criteria` or `update_context`, the tool implementation does this:

1. **Does NOT write** to `criterion` or `context.content_md`
2. **Validates** the proposed change (for example: the criterion name is not empty, the weight is 1–5)
3. **Returns** a result to the Agent loop:
   ```json
   {
     "status": "pending_confirmation",
     "summary": "Vorgeschlagen: Kriterium 'Nähe zum Wasser' (Gewicht 3). ⚠️ Noch nicht aktiv – Bestätigung ausstehend."
   }
   ```

The `status` field tells the Agent: „Do not use this for `evaluate_listing` in the current turn. Tell the user they need to confirm.“ The Agent's chat reply includes both the `summary` text and Claude's own explanation. The frontend shows a diff card next to the message, because the `tool_calls` array of the message holds the full proposal.

(The `summary` text means: "Proposed: criterion 'Nähe zum Wasser' (weight 3). Not active yet – waiting for confirmation.")

### Tool-Call Arguments as Source of Truth

Claude already puts the proposal into the tool call arguments as valid JSON:

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

These arguments are saved automatically in `chat_message.tool_calls` (JSONB) when the turn is persisted. There is no extra processing and no extraction step. They are already structured. Claude's tool-use schema already validated them. They are already versioned, because they are part of one specific chat message.

### Confirm / Reject Endpoints

There are two REST endpoints. Both are thin wrappers:

| Endpoint | Behaviour |
|---|---|
| `POST /api/chat/messages/{msg_id}/tool-calls/{call_idx}/confirm` | Reads `chat_message.tool_calls[call_idx]`. Checks it against the current state (no stale applies). Writes to the target table (`criterion` or `context`). Returns `200` and the updated state |
| `POST /api/chat/messages/{msg_id}/tool-calls/{call_idx}/reject` | Returns `200`. Rejecting means simply not confirming. This endpoint exists so the frontend can dismiss the card cleanly |

The frontend hides the confirm and reject buttons after either endpoint returns success. The diff card stays visible in the chat history. The buttons turn into a label ("Übernommen" / "Verworfen", meaning "accepted" / "rejected").

### Stale Detection

Between the proposal and the confirm, the user may have changed the affected entity by hand. On confirm, the endpoint checks:

- **For `propose_criteria`** (`action = "add"`): a criterion with the same name must not exist yet. If it does, it returns `409 Conflict`. The frontend then shows "Vorschlag veraltet — Kriterium existiert bereits." (means: "Proposal is outdated — criterion already exists.")
- **For `propose_criteria`** (`action = "edit"`): the referenced criterion must still exist. It must not have been changed by hand since the proposal. If it changed, it returns `409 Conflict` and the frontend shows "Vorschlag veraltet." (means: "Proposal is outdated.")
- **For `update_context`**: it always applies (appending or merging text). The current `content_md` is compared as a base. The proposal is a plain-text append, not a structured patch. If the base text changed, it returns `409 Conflict`. The frontend then shows the current text and the proposed text as a diff, so the user can decide.
- **For `reject`**: no check is needed. Accepting nothing is always safe.

There is no automatic merging and no diff3. This keeps it simple and predictable.

### Re-Evaluation After Confirm

After a Criteria proposal is confirmed (a new or changed Criterion), the backend starts a full re-evaluation of all Listings in the profile. This is exactly as described in [[../../spec/01_mvp/01-04-criteria-and-scoring]] and in the consequences of ADR 0004. The difference: the trigger is now the confirm endpoint, not a hidden side effect of the agent turn.

## Alternatives considered

- **A separate `pending_change` table** — rejected. It would copy proposal data that already exists in the `tool_calls` JSONB. It adds a schema migration, a repository, and a cleanup problem (when do we delete stale pending rows?).
- **A `status` column on `context_revision`** — rejected. `context_revision` is an append-only audit log that stores a snapshot before each change. If it also became a staging table, it would mix two jobs (history and proposals). It also would not cover `propose_criteria` at all.
- **Keep the pending state only in frontend memory** (no server-side storage; the client takes the tool call from the SSE stream, and the confirm POST sends the full proposal back) — rejected. It breaks easily on reload. Also, the server cannot check what is being applied unless it stores the proposal on the server. The `tool_calls` JSONB is that server-side store, with no extra infrastructure.

## Consequences

- `context_revision` stays a pure audit log. It has no `status` column and no pending rows. A revision is created only when `content_md` really changes (on confirm), not on proposal.
- The `update_context` and `propose_criteria` tool implementations are read-only for their target tables. They validate and return. They do not write.
- There are two light REST endpoints, about 30 lines each: validate, write, return.
- The frontend confirm-card component gets its data from the SSE stream (the tool-call arguments are already in the event). It does not use a separate pending-proposals API. The chat message *is* the API response.
- Stale detection is simple (check whether the target still exists and has not changed). It is not lossless: the user cannot automatically "merge" a stale proposal with manual edits. We accept this for the MVP. In a single-user app, the user sees the conflict, understands it, and asks the Agent again if needed.
