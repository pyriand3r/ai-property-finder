---
type: adr
title: "Chat history is repaired on load and each roundtrip is persisted atomically"
project: property-finder-obs-style
status: accepted
created: 2026-11-23
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. Extends [[0008-agent-turn-fire-and-forget]] and [[0010-tool-result-jsonb-for-replay]]. Affects [[../../spec/01_mvp/01-09-chat-reliability-fixes]].

## Context

Claude rejects a request with `400 tool_use ids were found without tool_result blocks immediately after` if any assistant `tool_use` is not answered by a `tool_result` in the next message. It also rejects a `tool_result` that answers nothing. The history is replayed in full on every turn. So one bad stored row breaks every later turn of that Chat Session for good (issue #1, [[../../buglist]]).

The loop itself never builds such a request in memory. The bad rows come from the persist path. `persistRoundtrip` wrote the assistant row and then each `role=tool` row as separate, non-atomic INSERTs, and only logged a failed INSERT. These paths leave a stored `tool_use` without its result:

1. The server stops between the assistant INSERT and the tool INSERTs (deploy, crash, OOM). ADR 0008 says there is no recovery on restart.
2. A tool INSERT fails and is swallowed. Example: `tool_result` is JSONB, so a result that is not valid JSON fails the INSERT.
3. Two turns write into one session at the same time (two tabs), so their rows interleave.
4. Rows written before RF.1 fixed the budget branch.

The reverse also happens: `toolCallsJSON` returned no `tool_calls` when a tool input was not valid JSON (the model hit its output limit mid-argument), while the tool rows were still written. That is an orphan `tool_result`.

## Decision

Do both, because they cover different cases:

1. **Repair on load (the guarantee).** `agent.RepairToolPairing` runs on the replayed history in `AgentService.runTurn`, after `Provider.FormatMessages`. A `tool_use` without a result gets a synthetic error result (`InterruptedToolResult`). An orphan `tool_result` is dropped. It works on the provider-neutral `Message` type (ADR 0007), is in memory only, and is idempotent. It also heals sessions that are already broken, which no write-side fix can do.
2. **Atomic persist (prevention).** `persistRoundtrip` writes the assistant row and all its tool rows in one transaction. `toolResultJSON` and `toolCallsJSON` always produce valid JSON, so an INSERT cannot fail on the content.

## Consequences

- No loop-budget or persist-semantics change (ADR 0013/0014/0016/0017). The turn budget is untouched. A stop still persists text only.
- If a roundtrip's transaction fails, that whole roundtrip is missing from history (before: a partial one). This is a clean gap, and replay stays valid.
- The model sees an error result for an interrupted call and decides itself whether to call the tool again.
- The DB rows of a broken session are not rewritten. The repair is applied again on every load.

## Alternatives considered

- **Only harden the persist path** — rejected. It cannot cover a stop between two writes of the process itself, and it leaves existing broken sessions broken.
- **Only repair on load** — rejected. It would hide failed writes: the UI would show a tool row that history replay treats as an orphan, and the cause would stay unfixed.
- **Delete or rewrite dangling rows in the DB** — rejected. ADR 0008 and ADR 0010 keep history lossless. An in-memory repair needs no migration.
- **Reject a second message while a turn runs (409)** — not done. The UI already disables input during a turn. The repair covers the multi-tab case.
