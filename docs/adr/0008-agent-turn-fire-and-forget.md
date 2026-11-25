---
type: adr
title: "Agent turns run to the end and are not recovered after a server restart"
project: property-finder-obs-style
status: accepted
created: 2026-08-01
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. This affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] and [[../../spec/00_infrastructure_setup/00-01-tech-stack]].

## Context

An Agent turn takes 30–120 seconds. It does these steps: URL → fetch → extract images → save → evaluate. During this time the user may close the browser tab. The SSE stream may drop. The server may restart.

The old spec of the predecessor project solved this with a full task persistence and recovery model. Every background job was a `task` row with a status, retries, and a recovery queue.

An Agent turn is not a background job. A synchronous user action starts it, and the user waits for the reply. The expensive work (LLM API calls) is already paid for when a failure might happen. If we threw that work away and ran the same LLM calls again on "recovery", the cost would double. The user would gain nothing.

The question: how much resilience is worth building for three failure cases? They differ a lot in how often they happen and how much they hurt.

## Decision

**No recovery mechanism. The Agent turn runs to the end or not at all.**

The pattern: start a goroutine with its own `context.Background()`. Let it run until the loop ends (all tool calls done, final Agent reply received). Then save the results as `chat_message` rows. The HTTP handler that started it returns right after it spawns the goroutine. The SSE stream is a separate listener that is not tied to the goroutine.

| Failure case | What happens | What the user sees on reload |
|---|---|---|
| Browser tab closed | The loop runs to the end in the background | All Agent messages are there, as if nothing happened |
| SSE stream dropped (network blip) | The loop runs to the end, and events pile up | The frontend reconnects SSE and gets the buffered events. If the stream is gone, it polls for the latest messages |
| Server restart | The loop dies with the process | The last user message is saved; the Agent reply is missing |

**The only hard requirement:** the user's message (`chat_message`, `role = 'user'`) is saved **before** the goroutine starts. It is not saved in a `defer`. If the server dies, the chat shows the user message with no reply, not a blank gap. The frontend has a small button "Antwort fehlt — erneut senden?" (means: "reply missing — send again?"). It lets the user start the turn again with one click.

There is no `task` table integration. There are no checkpoints in the middle of a turn. There is no recovery queue. We do not try to continue a half-finished turn after a restart. The LLM conversation state is lost with the process. To rebuild it, we would have to send all earlier messages again. That costs exactly as much as a fresh turn.

## Alternatives considered

- **Port the full `task` persistence model from the predecessor project** — rejected. That model was made for async background scraping jobs where the user is *not* waiting. In an Agent turn the user is in the loop by design. If the reply is missing, they will notice and try again. The task infrastructure is about 300 lines of code. It has a status column, a retry counter, a recovery goroutine, and a `task_retry_at` index. It would handle a case (server restart in the middle of a turn) that never happens in practice on a single-user Docker Compose setup.
- **Checkpoint after every tool call** (save partial state so the turn can resume) — rejected. To resume a turn after a restart, you would still have to replay the whole conversation history up to the crash point, because LLMs are stateless. That spends every token again. A fresh turn is cheaper and simpler.
- **SSE-with-postbox pattern** (POST returns 202, client polls for the result) — rejected. The user is actively waiting for the reply. Polling adds delay and complexity and gives no benefit.

## Consequences

- `internal/agent/loop.go` runs in a goroutine that is detached from the HTTP request lifecycle. The handler returns `202 Accepted` and the `chat_message.id` of the saved user message. The SSE stream then picks up the goroutine's output on its own.
- Chat turns do not use the `task` table. The `task` table is used for bulk re-evaluation and for portal searches. A portal search makes no LLM call, so it is resumed after a restart: see [[0028-portal-search-task-persistence]]. Chat turns stay unrecovered.
- A server restart destroys turns that are in progress, with no recovery. We accept this because it is extremely rare on a single-user Docker Compose app.
- The frontend needs exactly one new control: a retry button on messages where `role = 'user'` and no `role = 'assistant'` follow-up exists.
