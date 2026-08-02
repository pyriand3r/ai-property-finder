---
type: adr
title: "Agent turns run to completion, not recovered on server restart"
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

An Agent turn (URL → fetch → extract images → save → evaluate) takes 30–120 seconds. During that time the user's browser tab might be closed, the SSE stream might drop, or the server might restart. The old dev-property-finder spec solved this with a full task persistence/recovery model ([[../../../dev-property-finder/docs/adr/0004-task-persistence-and-recovery|ADR 0004]]): every background job was a `task` row with status, retries, and a recovery queue.

An Agent turn is not a background job — it's triggered by a synchronous user action, the user is waiting for the reply, and the expensive work (LLM API calls) has already been paid for by the time a failure might occur. Throwing away that work and re-running the same LLM calls on "recovery" would double the cost for no user-visible benefit.

The question: how much resilience mechanism is worth building for three failure scenarios that differ wildly in frequency and impact?

## Decision

**No recovery mechanism. The Agent turn runs to completion or not at all.**

The pattern: fire a goroutine with its own `context.Background()`, let it run until the loop ends (all tool calls done, final Agent reply received), then persist the results into `chat_message` rows. The HTTP handler that kicked it off returns immediately after spawning the goroutine — the SSE stream is a separate, decoupled listener.

| Failure scenario | What happens | User sees on reload |
|---|---|---|
| Browser tab closed | Loop runs to completion in background | All Agent messages present, as if nothing happened |
| SSE stream dropped (network blip) | Loop runs to completion, events pile up | Frontend reconnects SSE, receives buffered events, or polls latest messages if stream is gone |
| Server restart | Loop dies with the process | Last user message persisted; Agent reply missing |

**The only hard requirement:** the user's message (`chat_message`, `role = 'user'`) is persisted **before** the goroutine is spawned — not in a `defer`. If the server dies, the chat shows the user message with no reply, not a blank gap. A small "Antwort fehlt — erneut senden?" button on the frontend lets the user re-trigger with one click.

No `task` table integration. No checkpoints mid-turn. No recovery queue. No attempt to resume a half-finished turn after restart — the LLM conversation state is lost with the process and cannot be reconstructed without re-sending all prior messages anyway, which costs exactly as much as a fresh turn.

## Alternatives considered

- **Port the full `task` persistence model from dev-property-finder ADR 0004** — rejected: the model was designed for async background scraping jobs where the user is *not* waiting. An Agent turn has the user in the loop by construction; if the reply is missing, they'll notice and retry. The task infrastructure (status column, retry counter, recovery goroutine, `task_retry_at` index) is ~300 lines of code to handle a scenario (server restart mid-turn) that never happens in practice on a single-user Docker Compose deployment.
- **Checkpoint after every tool call** (save partial state so the turn can resume) — rejected: to resume a turn after restart, you'd need to replay the entire conversation history up to the crash point anyway (LLMs are stateless), which re-spends every token already spent. A fresh turn is cheaper and simpler.
- **SSE-with-postbox pattern** (POST returns 202, client polls for result) — rejected: the user is actively waiting for the reply; polling adds latency and complexity for no benefit.

## Consequences

- `internal/agent/loop.go` runs in a goroutine detached from the HTTP request lifecycle — the handler returns `202 Accepted` + the `chat_message.id` of the persisted user message, then the SSE stream picks up the goroutine's output independently
- No `task` table involvement for chat turns — the old `task` table remains only for Phase 3 automated recurring search, exactly where dev-property-finder ADR 0004 intended it
- Server restart destroys in-flight turns with no recovery — accepted as vanishingly rare on a single-user Docker Compose app
- The frontend needs exactly one new affordance: a retry button on messages where `role = 'user'` and no `role = 'assistant'` follow-up exists
