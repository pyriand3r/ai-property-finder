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

Accepted — affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]].

## Context

An Agent turn can involve an unbounded number of LLM API calls — each `fetch_url` result is fed back to the model, which may then call `save_listing`, then `evaluate_listing`, then `geocode`, each requiring another LLM roundtrip. Every roundtrip costs money (the full conversation history plus context files are sent as input tokens, and the model's response consumes output tokens). Without a budget, a single user message could trigger 15+ paid API calls.

The existing `max_tool_calls_per_turn` (15) counts tool dispatches, not LLM roundtrips. It was designed to prevent runaway tool-call loops, not to control cost — a turn with 14 `download_image` calls is 14 tool dispatches but only 1 LLM roundtrip, while a turn with `fetch_url → save_listing → evaluate_listing → propose_criteria` is 4 tool dispatches but 4 paid LLM roundtrips. Tool count and cost are not correlated.

Token-based budgeting (e.g. "max 100K tokens per turn") was considered: it directly measures cost. But it requires accurate token counting across providers (Anthropic and OpenAI count differently), adds complexity for PDF/document inputs (which consume large token blocks), and is hard for a user to reason about ("are 80K tokens a lot for this turn?"). Roundtrips are simple: each is one API call, one line on the bill, one decision point.

## Decision

**`llm.max_llm_roundtrips` (default 5) — the maximum number of LLM API calls per turn.**

```
Turn start
  │
  ├─ LLM call #1  →  Antwort: tool_calls [fetch_url, download_image×3]
  │                   Kosten: input (history + context) + output
  ├─ LLM call #2  →  Antwort: tool_calls [save_listing, evaluate_listing]
  │                   Kosten: input (history + context + tool results) + output
  ├─ LLM call #3  →  Antwort: tool_calls [geocode, save_listing]
  ├─ LLM call #4  →  Antwort: tool_calls [compute_ranking]
  └─ LLM call #5  →  Antwort: text only (fertig)
```

The counter increments once per `Provider.Chat()` call. After the response arrives:

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

The Agent receives this as a tool error (same error-handling path as any other tool failure), formulates a reply summarising progress, and asks whether to continue. The user can say "mach weiter" to start a new turn with fresh budget.

### Why 5 as default

A normal full ingestion flow: `fetch_url` → `save_listing` → `evaluate_listing` → `geocode` → `compute_ranking` = 5 roundtrips. Anything beyond that is unusual (e.g. the Agent following multiple secondary links, or re-fetching after a correction) and warrants asking the user before spending more.

### Relation to `max_tool_calls_per_turn`

Both limits coexist:

| Limit | Counts | Purpose |
|---|---|---|
| `max_tool_calls_per_turn` (15) | Tool dispatches | Prevents runaway loops; safety net |
| `max_llm_roundtrips` (5) | LLM API calls | Controls cost; user-facing budget |

A turn with one LLM call dispatching 14 `download_image` calls: hits neither limit (1 roundtrip, 14 tools — under 15). Fine.
A turn with 6 LLM calls dispatching 1 tool each: hits `max_llm_roundtrips` first. Asks user.

## Alternatives considered

- **Token-based budget** ("max 100K input + output tokens per turn") — rejected: requires accurate token counting per provider (Anthropic and OpenAI count differently), complicates PDF document handling (large input blocks), and is opaque to the user. Roundtrips are simpler to understand and implement.
- **Per-tool-type budget** ("max 3 LLM tool calls per turn", counting only `evaluate_listing` and `compute_ranking` as "LLM tools") — rejected: the Agent may legitimately need multiple non-LLM roundtrips (e.g. `fetch_url` on primary page, then `fetch_url` on broker page). Counting all roundtrips uniformly is simpler.
- **No budget at all, rely on user cancellation** — rejected: the user pastes a URL and walks away. Without a budget, a stuck or confused Agent burns money silently. The budget is a backstop, not a replacement for the user's stop button.

## Consequences

- `internal/agent/loop.go` gets a `roundtripCount` variable, incremented after each `Provider.Chat()` response. ~5 lines.
- Config: `llm.max_llm_roundtrips` (default 5) added to `app_config` schema.
- The "stop" control (`01-01`) remains for user-initiated cancellation. The budget is a safety net, not a user-facing feature — the user only sees it when it fires (Agent says "soll ich weitermachen?").
- `compute_ranking`'s internal LLM call (for Kurzfazit text generation) counts as a tool implementation detail, not a roundtrip — it runs inside the tool dispatch, not the loop. The budget only counts `Provider.Chat()` calls from the loop itself.
