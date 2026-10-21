---
type: adr
title: "The Agent loop is provider-independent, not tied to Anthropic's SDK"
project: property-finder-obs-style
status: accepted
created: 2026-08-01
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. It affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] and [[../../spec/00_infrastructure_setup/00-01-tech-stack]].

## Context

[[0006-single-provider-for-agent-mvp]] decided that the MVP ships with the Claude API as the only provider. A natural next question follows. Should we use Anthropic's own `ToolRunner` / Agent SDK to drive the multi-turn tool-use loop (`client.beta.messages.tool_runner`)? Or should we build the dispatch loop ourselves in `internal/agent/loop.go`?

Anthropic's SDK would handle these things:

- buffering partial tool calls
- sending results back into the conversation
- streaming deltas
- stopping the loop when the model returns only text

That saves real implementation work. But the SDK is Claude-specific. It uses Anthropic's event types, tool-call JSON schema, and streaming format. Every other provider (LM Studio, OpenCode) uses OpenAI-compatible tool-call formats. These differ in both HTTP schema and streaming event structure.

[[0006-single-provider-for-agent-mvp]] defers LM Studio and OpenCode. It does not drop them. So the question is not *whether* a second provider will come, but *when*. Building the loop directly on Anthropic's SDK would mean one of two things:

1. Rewrite the loop for each new provider (duplicated orchestration logic that is prone to drift), or
2. Map every provider's tool-call format *into* the internal shape of Anthropic's SDK (a leaky abstraction that breaks as soon as Anthropic extends its protocol).

## Decision

The agent loop (`internal/agent/loop.go`) is provider-independent orchestration code. It owns these jobs:

- building the system prompt from the preamble and context files
- sending the full message and tool history to the current provider
- receiving text deltas and tool-call requests in a provider-normalized form
- dispatching tool calls against the local tool registry
- feeding results back into the conversation
- stopping when the model returns no more tool calls

The `internal/agent.Provider` interface sits at the border between the loop and the LLM. It normalizes four things that differ across providers:

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

The `Provider` interface does **not** know what the tools do. It does **not** dispatch them. It does **not** decide whether to loop again. All of that belongs to `loop.go`.

In the Claude implementation (`internal/agent/provider/claude/`), the provider turns Anthropic's SSE event types into our normalized event stream. These types are `content_block_delta`, `content_block_stop` for tool calls, and `message_delta` for usage. Future LM Studio / OpenCode implementations will implement the same interface against OpenAI-compatible streaming tool-use events.

The cost of this abstraction is an extra translation layer (~100–150 lines of event-mapping code per provider). The benefit is that `loop.go` never changes when a new provider is added. In particular, it never has to learn a second tool-call JSON schema or streaming format.

## Alternatives considered

- **Build directly on Anthropic's SDK for MVP, abstract later** — rejected. The loop is the core of the application. Rewriting it when a second provider arrives would touch every integration point at once: tool dispatch, error handling, the SSE bridge to the frontend, cancellation, and token tracking. Such a rewrite causes regressions in exactly the place where correctness matters most.
- **Abstract at a higher level — make the whole loop pluggable** — rejected. With a loop per provider, every provider ships its own orchestration logic. That includes the "stop condition," "what to do on tool error," and "ordering guarantees." These are application-level rules, not provider-level rules. Duplicating them guarantees drift.
- **Use the OpenAI-compatible format as the main format** (LM Studio and OpenCode both speak it, and Anthropic has an OpenAI-compatible endpoint) — considered but rejected for MVP. Anthropic's native format has richer tool-use metadata (for example `server_tool_use` vs `tool_use`). The OpenAI-compatible proxy may also lag behind native features. One extra provider implementation costs little enough that we do not need this shortcut.

## Consequences

- `internal/agent/loop.go` is ~200–400 lines of orchestration code that must be correct. It owns the tool-dispatch loop, error handling, cancellation, and the streaming bridge.
- `internal/agent.Provider` is the seam that shields the loop from provider-specific wire formats. It is implemented once per provider and tested on its own.
- The Claude-specific provider (`internal/agent/provider/claude/`) maps Anthropic's streaming SSE events to our normalized `ChatEvent` stream. This mapping is the only provider-specific code that exists at MVP time.
- `internal/agent/tools.go` defines the Tool struct and the local dispatch registry. It is unchanged from the original spec. It is now clearer that the loop owns it, not the provider.
- The frontend SSE bridge (`useChatStream`) receives `ChatEvent` equivalents serialized as JSON over the existing SSE channel. The streaming transport from loop to frontend is provider-independent from day one.
