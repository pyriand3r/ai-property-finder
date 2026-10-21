---
type: adr
title: "Agent-turn budget: max LLM roundtrips, not per-tool-call or token-based"
project: property-finder-obs-style
status: accepted
created: 2026-08-01
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. It affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]].

## Context

An Agent turn can make any number of LLM API calls. The result of each `fetch_url` goes back to the model. The model may then call `save_listing`, then `evaluate_listing`, then `geocode`. Each of these needs another LLM roundtrip. Every roundtrip costs money: the full conversation history plus the context files are sent as input tokens, and the model's answer uses output tokens. Without a budget, one user message could cause 15 or more paid API calls.

The existing `max_tool_calls_per_turn` (15) counts tool dispatches, not LLM roundtrips. It was made to stop runaway tool-call loops, not to control cost. A turn with 14 `download_image` calls is 14 tool dispatches but only 1 LLM roundtrip. A turn with `fetch_url → save_listing → evaluate_listing → propose_criteria` is 4 tool dispatches but 4 paid LLM roundtrips. The number of tools and the cost do not match.

We also considered a token-based budget (for example "max 100K tokens per turn"). It measures cost directly. But it needs exact token counting across providers (Anthropic and OpenAI count differently). It is harder with PDF and document inputs, which use large token blocks. It is also hard for a user to judge ("are 80K tokens a lot for this turn?"). Roundtrips are simple: each one is one API call, one line on the bill, and one decision point.

## Decision

**`llm.max_llm_roundtrips` (default 5) is the maximum number of LLM API calls per turn.**

```
Turn start
  │
  ├─ LLM call #1  →  Answer: tool_calls [fetch_url, download_image×3]
  │                   Cost: input (history + context) + output
  ├─ LLM call #2  →  Answer: tool_calls [save_listing, evaluate_listing]
  │                   Cost: input (history + context + tool results) + output
  ├─ LLM call #3  →  Answer: tool_calls [geocode, save_listing]
  ├─ LLM call #4  →  Answer: tool_calls [compute_ranking]
  └─ LLM call #5  →  Answer: text only (done)
```

The counter goes up by one for each `Provider.Chat()` call. After the response arrives:

```go
roundtripCount++
if roundtripCount >= maxRoundtrips && response.HasToolCalls() {
    // Don't dispatch the tool calls. Return an error to the model:
    return ToolError{
        ToolCallID: response.ToolCalls[0].ID,
        Message:    "Maximale Anzahl Anfragen erreicht. Sage dem User was du geschafft hast und frag ob du weitermachen sollst.",
    }
}
```

The message text is in German. It means: "Maximum number of requests reached. Tell the user what you have done and ask if you should continue."

The Agent gets this as a tool error (the same error path as any other tool failure). It writes a reply that sums up its progress and asks whether to continue. The user can say "mach weiter" (meaning: keep going) to start a new turn with a fresh budget.

### Why 5 as default

A normal full ingestion flow is `fetch_url` → `save_listing` → `evaluate_listing` → `geocode` → `compute_ranking`. That is 5 roundtrips. Anything more is unusual (for example, the Agent follows several secondary links, or fetches again after a correction). In that case it is right to ask the user before spending more.

### Relation to `max_tool_calls_per_turn`

Both limits exist together:

| Limit | Counts | Purpose |
|---|---|---|
| `max_tool_calls_per_turn` (15) | Tool dispatches | Stops runaway loops; safety net |
| `max_llm_roundtrips` (5) | LLM API calls | Controls cost; budget the user can see |

A turn with one LLM call that dispatches 14 `download_image` calls hits neither limit (1 roundtrip, 14 tools, under 15). This is fine.
A turn with 6 LLM calls that dispatch 1 tool each hits `max_llm_roundtrips` first. The Agent asks the user.

## Alternatives considered

- **Token-based budget** ("max 100K input + output tokens per turn"): rejected. It needs exact token counting for each provider (Anthropic and OpenAI count differently). It makes PDF document handling harder (large input blocks). The user cannot easily understand it. Roundtrips are easier to understand and to build.
- **Per-tool-type budget** ("max 3 LLM tool calls per turn", counting only `evaluate_listing` and `compute_ranking` as "LLM tools"): rejected. The Agent may need several roundtrips that do not use an LLM tool. One example is `fetch_url` on the main page, then `fetch_url` on the broker page. Counting all roundtrips the same way is simpler.
- **No budget at all, rely on the user to cancel**: rejected. The user pastes a URL and walks away. Without a budget, a stuck or confused Agent spends money without anyone noticing. The budget is a backstop. It does not replace the user's stop button.

## Consequences

- `internal/agent/loop.go` gets a `roundtripCount` variable. It goes up after each `Provider.Chat()` response. This is about 5 lines.
- Config: `llm.max_llm_roundtrips` (default 5) is added to the `app_config` schema.
- The "stop" control (`01-01`) stays for cancelling by the user. The budget is a safety net, not a feature the user works with. The user only sees it when it fires (the Agent says "soll ich weitermachen?", meaning: should I continue?).
- The internal LLM call of `compute_ranking` (to write the Kurzfazit text, the short summary) is an implementation detail of the tool. It does not count as a roundtrip. It runs inside the tool dispatch, not in the loop. The budget only counts `Provider.Chat()` calls made by the loop itself.
