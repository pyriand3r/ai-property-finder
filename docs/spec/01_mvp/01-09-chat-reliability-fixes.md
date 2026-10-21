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

Two reliability bugs were found in a hands-on review after Layer 6 (Chat/Agent) shipped. Both come from the same mechanism: how `chat_message.tool_calls` is saved for each roundtrip and replayed into the history of the next turn. These are not new features. They are corrections to Layer 6.5/6.3 behaviour.

---

## Confirm-Card status must survive reload

**Symptom**: The user already clicked Übernehmen or Verwerfen (means: "accept" or "discard") on a `propose_criteria`/`update_context` Confirm-Card. After the user leaves the Chat tab and returns, or reloads the page, the card shows the button pair again, as if it was never actioned.

**Root cause**: The `status` ref in `ChatConfirmCard.vue` is local component state. It is hardcoded to `'pending'` on every mount. Nothing on the server ever recorded how a proposal was resolved.

**Fix**: [[../../docs/adr/0017-tool-call-status-in-jsonb]]. A `status` field is added to the `StoredToolCall` shape that is already stored in `tool_calls`. The confirm/reject endpoints set it. The card reads it once on mount. See that ADR for the full design and the alternatives considered.

As a side effect, Confirm/Reject become idempotent. Acting on an already-actioned call now returns `409 Conflict`. Before, it silently re-applied the call (an `update_context` append) or did nothing (a second reject).

---

## "weitermachen" after hitting the roundtrip limit must actually continue

**Symptom**: The Agent hits `llm.max_llm_roundtrips` in the middle of a turn, while the model has tool calls in flight. The Agent replies with the limit-reached note and asks whether to continue. The user replies "weitermachen" (means: "continue"). The Agent's next turn then does nothing useful.

**Root cause**: The budget can run out in [[../../docs/adr/0013-agent-turn-llm-roundtrip-budget|the roundtrip-budget branch]] of `Run()` in `internal/agent/loop.go`. In that case, the assistant message saved for that roundtrip includes the `tool_use` blocks the model had just requested. But those calls were **never dispatched** (the budget ran out before `dispatchAll` runs). So no matching `role=tool` result row exists. On the next turn, the replayed history has an assistant `tool_use` with no `tool_result` in the next message. Claude's API requires every `tool_use` to be followed at once by its result. It rejects a history that breaks this rule. Depending on the exact API behaviour, this either fails the very next roundtrip outright, or leaves the model unable to make sense of a "half-requested" tool call whose outcome it cannot see. Either way, it cannot continue in a useful way.

**Fix**: In the `budgetExhausted` branch, the saved assistant message is built from **text only**. That text is the roundtrip's own text plus the limit-reached note. The `tool_use` blocks are left out for calls that were requested but never dispatched. The note text already tells the model (and the user) that a cutoff happened. A later "weitermachen" then starts a clean roundtrip. The model decides again what to call. It does not try to resume a plan whose result it can no longer see. This matches the preamble's own instruction for this case ("fasse den Stand zusammen und frag, ob weitergemacht werden soll", means: "summarize the state and ask whether to continue"). The Agent should summarize and ask. It should not silently retry a stale, half-formed call.

---

## Verification

Both fixes were tested live against the real Claude API, not only with unit tests. Unit tests exist for the pure logic. But the real API rejection behaviour can only be confirmed against the real service.

- Confirm a proposal. Reload the page (or switch tabs and back). Check that the card shows "Übernommen"/"Verworfen" (means: "accepted"/"discarded") instead of the button pair. Send the confirm/reject POST again on an already-actioned call. Check that it returns `409`.
- Lower `llm.max_llm_roundtrips` in `app_config` for a short time, so the limit triggers. Ask the Agent something that needs more roundtrips than the limit. Reply "weitermachen" to the resulting note. Check that the Agent really continues and does not stall.
