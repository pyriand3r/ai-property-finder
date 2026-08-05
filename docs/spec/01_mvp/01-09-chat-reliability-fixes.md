---
type: spec
created: 2026-08-05
title: Chat Reliability Fixes
project: property-finder-obs-style
spec: chat-reliability-fixes
phase: 1
order: 9
category: chat
status: ready
updated: 2026-08-05
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - chat
  - agent
---

## Overview

Two reliability bugs found during a hands-on review pass after Layer 6 (Chat/Agent) shipped, both rooted in the same mechanism: how `chat_message.tool_calls` gets persisted per roundtrip and replayed into the next turn's history. Not new features — corrections to Layer 6.5/6.3 behavior.

---

## Confirm-Card status must survive reload

**Symptom**: a `propose_criteria`/`update_context` Confirm-Card the user already clicked Übernehmen/Verwerfen on shows the button pair again after leaving the Chat tab and returning, or after a page reload — as if never actioned.

**Root cause**: `ChatConfirmCard.vue`'s `status` ref is local component state, hardcoded to `'pending'` on every mount. Nothing server-side ever recorded which way a proposal was resolved.

**Fix**: [[../../docs/adr/0017-tool-call-status-in-jsonb]] — a `status` field added to the `StoredToolCall` shape already stored in `tool_calls`, set by the confirm/reject endpoints, read once by the card on mount. See that ADR for the full design and the alternatives considered.

Confirm/Reject also become idempotent as a side effect: acting on an already-actioned call now returns `409 Conflict` instead of silently re-applying (an `update_context` append) or no-op'ing (a second reject).

---

## "weitermachen" after hitting the roundtrip limit must actually continue

**Symptom**: the Agent hits `llm.max_llm_roundtrips` mid-turn while the model has tool calls in flight, replies with the limit-reached note asking whether to continue, the user replies "weitermachen" — and the Agent's next turn does nothing productive.

**Root cause**: when the budget is exhausted in [[../../docs/adr/0013-agent-turn-llm-roundtrip-budget|the roundtrip-budget branch]] of `internal/agent/loop.go`'s `Run()`, the assistant message persisted for that roundtrip includes the `tool_use` blocks the model had just requested — but those calls were **never dispatched** (the budget ran out before `dispatchAll` runs), so no matching `role=tool` result row exists. On the next turn, the replayed history contains an assistant `tool_use` with no `tool_result` in the following message — Claude's API requires every `tool_use` to be immediately followed by its result and rejects a history that violates this. Depending on exact API behavior this either fails the very next roundtrip outright or leaves the model unable to make sense of a "half-requested" tool call it can't see the outcome of, either way it can't productively continue.

**Fix**: in the `budgetExhausted` branch, the persisted assistant message is built from **text only** — the roundtrip's own text plus the limit-reached note — omitting `tool_use` blocks for calls that were requested but never dispatched. The note text already tells the model (and the user) that a cutoff happened; a subsequent "weitermachen" then starts a clean roundtrip and the model re-decides what to call, rather than trying to resume a plan it can no longer see the result of. This matches the preamble's own instruction for this situation ("fasse den Stand zusammen und frag, ob weitergemacht werden soll") — summarize and ask, don't silently retry a stale half-formed call.

---

## Verification

Both fixed live against the real Claude API, not just unit-tested (unit tests exist for the pure logic, but the actual API rejection behavior can only be confirmed against the real service):

- Confirm a proposal, reload the page (or switch tabs and back), verify the card shows "Übernommen"/"Verworfen" instead of the button pair. Re-POST confirm/reject on an already-actioned call, verify `409`.
- Temporarily lower `llm.max_llm_roundtrips` in `app_config` to force the limit to trigger, ask the Agent something requiring more roundtrips than the limit, reply "weitermachen" to the resulting note, verify the Agent actually continues instead of stalling.
