---
type: adr
title: "LM Studio and OpenCode run through one OpenAI-compatible Provider"
project: property-finder-obs-style
status: accepted
created: 2026-11-25
tags:
  - property-finder-obs-style
  - adr
  - agent
---

## Status

Accepted. It implements the revisit trigger of [[0006-single-provider-for-agent-mvp]] and uses the seam of [[0007-agent-loop-provider-independent]]. It affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Provider Abstraction]]. Issue #22.

## Context

ADR 0006 shipped Claude only and reserved `lmstudio` and `opencode` as `llm.provider` values. The MVP is complete, so the Agent loop is proven. LM Studio and OpenCode both expose the OpenAI Chat Completions API (`POST {base_url}/chat/completions`, SSE streaming, `tools`). Neither needs a different wire format.

## Decision

- One package, `internal/agent/provider/openai`, implements `agent.Provider` for both. It uses plain `net/http` and an SSE reader. It adds **no SDK dependency**. The two providers differ only in the catalogue entry in `llm.Providers()` (label, default Base URL, whether an API key is required).
- `llm.buildSnapshot` builds this provider for `lmstudio` and `opencode`. It builds the unconfigured stand-in when the Base URL is empty. A key is **not** required: a local LM Studio has none. The key is sent as `Authorization: Bearer` only when set.
- The single-shot `ai.Provider` (evaluation, ranking, rejection analysis) gets a matching non-streaming transport (`internal/ai/openai.go`) behind the existing `completer` interface. Otherwise a user who selects LM Studio would get a working chat but failing evaluations.
- Base URL is required for these providers (validated in `llm.validate`) and must include the version prefix, for example `http://localhost:1234/v1`. `DefaultBaseURL` in the catalogue is only a UI placeholder. It is never applied silently.
- Capabilities: `SupportsDocumentInput()` and `SupportsImageInput()` are both `false`. This is the capability flag that ADR 0005/0006 asked for. A linked PDF becomes a tool error (existing behaviour in `loop.go`). An attached image is not sent. Reason for images: only vision models accept them, and the config cannot say whether the loaded model is one.
- `FormatMessages` is shared: the stored-row-to-message mapping moved to `agent.StoredHistoryMessages`. The Claude provider uses it too. The OpenAI split (one `role: "tool"` message per result) happens in `Chat`.
- A stream that ends without `[DONE]` or a `finish_reason` is an error. Tool calls without an ID get a generated one. A parameterless call gets `{}` as arguments.

## Alternatives considered

- **Use an OpenAI SDK.** Rejected. It adds a dependency for one endpoint, and the local servers differ in small ways that the SDK types do not tolerate well (missing IDs, missing `[DONE]`).
- **Two packages, one per backend.** Rejected. They would be identical code.
- **Only the agent `Provider`, leave `ai.Provider` unconfigured.** Rejected, see Decision.
- **Make images work.** Deferred until there is a per-provider or per-model switch.

## Consequences

- Tool-use quality depends on the model the user loads. ADR 0006 warned about this. It is not tested here beyond the wire format. Real behaviour with a 12-tool catalogue must be judged in use.
- Token usage comes from the final `usage` chunk (`stream_options.include_usage`). A server that does not send it reports 0 tokens, and the token donut stays empty (ADR 0014).
- The Base URL is one shared setting. After switching provider the user must set the matching Base URL.
