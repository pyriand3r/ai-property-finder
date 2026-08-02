---
type: adr
title: "Agent loop is built provider-independently, not tied to Anthropic's SDK"
project: property-finder-obs-style
status: accepted
created: 2026-08-01
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/00_infrastructure_setup/00-01-tech-stack]].

## Context

[[0006-single-provider-for-agent-mvp]] decided that MVP ships with Claude API as the sole provider. A natural follow-up question is whether to use Anthropic's own `ToolRunner` / Agent SDK to drive the multi-turn tool-use loop (`client.beta.messages.tool_runner`), or to build the dispatch loop ourselves in `internal/agent/loop.go`.

Anthropic's SDK would handle: buffering partial tool calls, dispatching results back into the conversation, streaming deltas, and stopping the loop when the model returns only text. That is real implementation weight saved. But the SDK is Claude-specific — it speaks Anthropic's event types, tool-call JSON schema, and streaming format. Every alternative provider (LM Studio, OpenCode) uses OpenAI-compatible tool-call formats, which differ in both HTTP schema and streaming event structure.

Since [[0006-single-provider-for-agent-mvp]] explicitly defers LM Studio and OpenCode — not drops them — the question is not *whether* a second provider will arrive, but *when*. Building the loop directly on Anthropic's SDK would mean either:

1. Rewriting the loop for each new provider (duplicated, drift-prone orchestration logic), or
2. Mapping every provider's tool-call format *into* Anthropic's SDK-internal shape (a leaky abstraction that breaks the moment Anthropic extends their protocol).

## Decision

The agent loop (`internal/agent/loop.go`) is built as provider-independent orchestration code. It owns: assembling the system prompt from preamble + context files, sending the full message/tool history to the current provider, receiving text deltas and tool-call requests in a provider-normalized form, dispatching tool calls against the local tool registry, feeding results back into the conversation, and stopping when the model returns no further tool calls.

The `internal/agent.Provider` interface exists at the boundary between loop and LLM. It normalizes four things that differ across providers:

```go
type Provider interface {
    // Does this provider accept document (PDF) content blocks natively?
    SupportsDocumentInput() bool

    // Convert app-level Tool definitions into the provider's expected JSON schema format.
    // Called once per turn when assembling the request.
    FormatTools(tools []Tool) any

    // Send a chat completion request (messages + tools) and receive a stream of events.
    // The stream emits text deltas, tool-call requests, and a usage summary on close.
    // Returns a channel so the loop can select between stream events and context cancellation.
    Chat(ctx context.Context, req ChatRequest) (<-chan ChatEvent, error)
}

type ChatRequest struct {
    System   []ContentBlock // assembled preamble + context files + listing/user_confirmed_fields context
    Messages []Message      // full session history (user, assistant, tool_result)
    Tools    []Tool
    Stream   bool
}

type ChatEvent struct {
    Type       ChatEventType // text_delta | tool_call_request | tool_call_delta | done
    Text       string
    ToolCall   *ToolCall     // set for tool_call_request, nil for other types
    Usage      *Usage        // set on done
    Error      error         // set on error
}

type ToolCall struct {
    ID        string
    Name      string
    Arguments json.RawMessage
}
```

The `Provider` interface does **not** know what the tools do, does **not** dispatch them, and does **not** decide whether to loop again — those all belong to `loop.go`.

For the Claude implementation (`internal/agent/provider/claude/`): the provider translates our normalized event stream from Anthropic's SSE event types (`content_block_delta`, `content_block_stop` for tool calls, `message_delta` for usage). For future LM Studio / OpenCode implementations: the same interface is implemented against OpenAI-compatible streaming tool-use events.

The cost of this abstraction is the extra translation layer (~100–150 lines of event-mapping code per provider). The benefit is that `loop.go` never changes when a new provider is added — and specifically never needs to learn a second tool-call JSON schema or streaming format.

## Alternatives considered

- **Build directly on Anthropic's SDK for MVP, abstract later** — rejected: the loop is the core of the application. Rewriting it when a second provider arrives would touch every integration point (tool dispatch, error handling, SSE bridge to frontend, cancellation, token tracking) simultaneously. That kind of rewrite is a source of regressions in exactly the place where correctness matters most.
- **Abstract at a higher level — make the whole loop pluggable** — rejected: a loop-per-provider means every provider ships its own orchestration logic, including "stop condition," "what to do on tool error," "ordering guarantees." Those are application-level semantics, not provider-level; duplicating them guarantees drift.
- **Use the OpenAI-compatible format as the canonical format** (since LM Studio and OpenCode both speak it, and Anthropic has an OpenAI-compatible endpoint) — considered but rejected for MVP: Anthropic's native format has richer tool-use metadata (e.g. `server_tool_use` vs `tool_use`) and the OpenAI-compatible proxy may lag behind native features. The abstraction cost of one extra provider implementation is low enough to not need this shortcut.

## Consequences

- `internal/agent/loop.go` is ~200–400 lines of orchestration code that must be correct — the tool-dispatch loop, error handling, cancellation, and streaming bridge are all owned here
- `internal/agent.Provider` is the seam that insulates the loop from provider-specific wire formats — implemented once per provider, tested independently
- The Claude-specific provider (`internal/agent/provider/claude/`) maps Anthropic's streaming SSE events to our normalized `ChatEvent` stream — this mapping is the only provider-specific code that exists at MVP time
- `internal/agent/tools.go` defines the Tool struct and the local dispatch registry — unchanged from the original spec, but now clearer that it's loop-owned, not provider-owned
- The frontend SSE bridge (`useChatStream`) receives `ChatEvent` equivalents serialized as JSON over the existing SSE channel — the streaming transport from loop to frontend is provider-independent from day one
