---
type: adr
title: "Tool results are stored as JSONB in chat_message.tool_result and mapped per provider on replay"
project: property-finder-obs-style
status: accepted
created: 2026-08-01
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. This meets the history replay need of multi-turn Agent conversations. It affects [[../../spec/00_infrastructure_setup/00-02-database-schema]] and [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]].

## Context

The Agent loop in [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Agent Loop]] sends the "full session history" to the LLM on every turn. An LLM with tool use needs correctly paired `tool_use` and `tool_result` messages. The tool result must be the exact JSON the tool returned, not a human-readable summary.

The current `chat_message` schema stores:
- `tool_calls` (JSONB) — the tool-use requests from the assistant
- `content` (TEXT) — for `role = 'tool'`, a "human-readable summary" of the result

The raw tool result JSON is **lost** after the turn is saved. When the next user message arrives, the loop rebuilds the history for the LLM. It then has no valid `tool_result` content to send back.

We looked at three approaches. We could store raw results in a separate column. We could rebuild them from the summary, but that loses data. We could skip tool turns when replaying, but that breaks multi-turn context.

[[0007-agent-loop-provider-independent]] requires the loop and the DB to be provider-independent. So the stored format must be neutral. It must not be Claude's `content_block` structure or OpenAI's string-wrapped JSON. The mapping to each provider's wire format happens at the `Provider` level on replay.

## Decision

**One new column: `chat_message.tool_result` (JSONB).**

### Schema

```sql
ALTER TABLE chat_message ADD COLUMN tool_result JSONB;
-- Set when role = 'tool'. NULL for all other roles.
-- Shape: {"tool_call_id": "<id>", "result": <any>}
```

### Write Path (during Agent loop)

When the loop runs a tool call and gets a result:

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

(The summary text means: "Listing saved: Kappmuren 2, 310,000 kr".)

### Read Path (history replay for next turn)

To build the message array for the LLM, `context_builder.go` goes through the `chat_message` rows. It passes them to the current `Provider` to translate:

```
DB row (role=tool):
  content     = "Inserat gespeichert: Kappmuren 2, 310.000 kr"   → Frontend rendering
  tool_result = {"tool_call_id":"call_1","result":{"status":"ok",...}}  → LLM replay

Provider.Chat() receives:  []Message{...this row...}
Provider translates into:  Claude tool_result content block, OR OpenAI tool role message
```

The `content` column is only used to show text in the frontend. The LLM never sees it. The LLM sees the `tool_result` after the provider has mapped it.

### Provider-specific mapping

| Provider | `tool_result` DB value | Sent to LLM as |
|---|---|---|
| Claude | `{"tool_call_id":"call_1","result":{...}}` | `role: "user"`, `content: [{type:"tool_result", tool_use_id:"call_1", content:[{type:"text", text:"{...}"}]}]` |
| OpenAI | same | `role: "tool"`, `tool_call_id: "call_1"`, `content: "{...}"` (stringified JSON) |
| LM Studio | same | same as OpenAI (OpenAI-compatible endpoint) |

The mapping is about 20 lines per provider. The DB and the loop never know which format the LLM expects. That is the boundary of the `Provider` interface.

## Alternatives considered

- **No extra column — rebuild tool results from the human-readable summary** — rejected. This loses data. "Inserat gespeichert: Kappmuren 2, 310.000 kr" does not contain the `listing_id` UUID, the filter values, or any structured data the next turn might need.
- **Skip tool-result turns when replaying history** — rejected. It breaks context. Say the Agent called `save_listing` two turns ago. The user now asks "compare this to the one we just saved". The LLM needs the `listing_id` from that tool call to call `list_profile_listings` correctly.
- **Store the provider-specific format directly** (Claude content blocks in the DB) — rejected. It breaks the provider-independence goal of [[0007-agent-loop-provider-independent]]. LM Studio cannot read Claude content blocks.
- **Store in a separate `tool_message` table** (1:N from chat_message) — rejected. Tool results are 1:1 with tool calls (one call, one result). A separate table adds a join and gives no benefit. The JSONB column is simpler.

## Consequences

- `chat_message` gains one column. The schema migration is `ALTER TABLE ... ADD COLUMN tool_result JSONB;`
- `context_builder.go` hands message formatting to `Provider.FormatMessages()`. It no longer builds raw message arrays itself.
- The `Provider` interface gains one method: `FormatMessages([]db.ChatMessage) []provider.Message`. It converts DB-neutral rows into provider-specific LLM message structures.
- The `content` column stays human-readable for the frontend. The `tool_result` column is machine-readable for the LLM. The two jobs stay clearly apart.
- Future providers (OpenCode, LM Studio) implement exactly one mapping function. They do not need a separate history store.
