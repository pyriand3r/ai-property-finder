---
type: adr
title: "Tool results stored as JSONB in chat_message.tool_result, provider-mapped on replay"
project: property-finder-obs-style
status: accepted
created: 2026-08-01
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — implements the history replay requirement for multi-turn Agent conversations. Affects [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]].

## Context

The Agent loop per [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Agent Loop]] sends the "full session history" to the LLM on every turn. An LLM that supports tool use requires correctly paired `tool_use`/`tool_result` messages — the tool result must be the exact JSON the tool returned, not a human-readable summary.

The current `chat_message` schema stores:
- `tool_calls` (JSONB) — the tool-use requests from the assistant
- `content` (TEXT) — for `role = 'tool'`, a "human-readable summary" of the result

The raw tool result JSON is **lost** after persisting the turn. When the next user message arrives and the loop reconstructs history for the LLM, it has no valid `tool_result` content to feed back.

Three approaches were considered: store raw results in a separate column, reconstruct them from the summary (not lossless), or skip tool turns entirely when replaying (breaks multi-turn context).

Since [[0007-agent-loop-provider-independent]] requires the loop and DB to be provider-independent, the stored format must be neutral — not Claude's `content_block` structure or OpenAI's string-wrapped JSON. The mapping to each provider's wire format happens at the `Provider` level on replay.

## Decision

**One new column: `chat_message.tool_result` (JSONB).**

### Schema

```sql
ALTER TABLE chat_message ADD COLUMN tool_result JSONB;
-- Set when role = 'tool'. NULL for all other roles.
-- Shape: {"tool_call_id": "<id>", "result": <any>}
```

### Write Path (during Agent loop)

When the loop dispatches a tool call and gets a result:

```go
// In internal/agent/loop.go, after tool dispatch:
resultJSON, _ := json.Marshal(toolResult)
msg := ChatMessage{
    Role:       "tool",
    Content:    toolResult.Summary(),           // "Inserat gespeichert: Kappmuren 2, 310.000 kr"
    ToolResult: json.RawMessage(fmt.Sprintf(    // {"tool_call_id":"call_1","result":{...}}
        `{"tool_call_id":"%s","result":%s}`, callID, resultJSON)),
}
```

### Read Path (history replay for next turn)

When assembling the message array for the LLM, `context_builder.go` iterates `chat_message` rows and passes them to the current `Provider` for translation:

```
DB row (role=tool):
  content     = "Inserat gespeichert: Kappmuren 2, 310.000 kr"   → Frontend rendering
  tool_result = {"tool_call_id":"call_1","result":{"status":"ok",...}}  → LLM replay

Provider.Chat() receives:  []Message{...this row...}
Provider translates into:  Claude tool_result content block, OR OpenAI tool role message
```

The `content` column is only used for frontend display. The LLM never sees it — it sees the provider-mapped `tool_result`.

### Provider-specific mapping

| Provider | `tool_result` DB value | Sent to LLM as |
|---|---|---|
| Claude | `{"tool_call_id":"call_1","result":{...}}` | `role: "user"`, `content: [{type:"tool_result", tool_use_id:"call_1", content:[{type:"text", text:"{...}"}]}]` |
| OpenAI | same | `role: "tool"`, `tool_call_id: "call_1"`, `content: "{...}"` (stringified JSON) |
| LM Studio | same | same as OpenAI (OpenAI-compatible endpoint) |

The mapping is ~20 lines per provider. The DB and the loop never know which format the LLM expects — that's the `Provider` interface boundary.

## Alternatives considered

- **No extra column — reconstruct tool results from the human-readable summary** — rejected: not lossless. "Inserat gespeichert: Kappmuren 2, 310.000 kr" doesn't contain the `listing_id` UUID, filter values, or any structured data the next turn might need.
- **Skip tool-result turns when replaying history** — rejected: breaks context. If the Agent called `save_listing` two turns ago and the user now asks "compare this to the one we just saved," the LLM needs to know the `listing_id` from that tool call to call `list_profile_listings` correctly.
- **Store provider-specific format directly** (Claude content blocks in the DB) — rejected: violates the provider-independence goal of [[0007-agent-loop-provider-independent]]. LM Studio can't read Claude content blocks.
- **Store as a separate `tool_message` table** (1:N from chat_message) — rejected: tool results are 1:1 with tool calls (one call, one result). A separate table adds a join for no benefit. The JSONB column is simpler.

## Consequences

- `chat_message` gains one column — schema migration is `ALTER TABLE ... ADD COLUMN tool_result JSONB;`
- `context_builder.go` delegates message formatting to `Provider.FormatMessages()` instead of building raw message arrays itself
- `Provider` interface gains one method: `FormatMessages([]db.ChatMessage) []provider.Message` — converts DB-neutral rows into provider-specific LLM message structures
- The `content` column remains human-readable for the frontend; `tool_result` is machine-readable for the LLM — clear separation of concerns
- Future providers (OpenCode, LM Studio) implement exactly one mapping function, not a separate history store
