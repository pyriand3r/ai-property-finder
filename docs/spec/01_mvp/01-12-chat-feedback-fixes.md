---
type: spec
created: 2026-08-05
title: Chat Feedback Fixes
project: property-finder-obs-style
spec: chat-feedback-fixes
phase: 1
order: 12
category: chat
status: ready
updated: 2026-08-05
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - chat
---

## Overview

Two more issues found live-testing [[01-09-chat-reliability-fixes]]'s fixes. Neither is a data-correctness bug like 01-09's — both are chat-UI feedback gaps that make a working Agent turn *look* broken.

---

## Turn-In-Progress Indicator

**Symptom**: after sending a message (including a "weitermachen" continuation), nothing visible happens until the first `chat.text_delta`/`chat.tool_progress` SSE event arrives. For a turn that starts with a slow tool call before any text, this reads as "did nothing" — exactly the confusion 01-09's "weitermachen" fix was trying to eliminate the *cause* of, but the symptom (silence) can still happen for an entirely healthy turn that's just taking a few seconds to start.

**Fix**: `useChat.ts` gains a state flag — set the moment `send()`'s `POST` succeeds (the turn has been kicked off server-side, per ADR 0008 fire-and-forget), cleared the moment the *first* live signal arrives for that turn: a `chat.text_delta`, a `chat.tool_progress`, the terminal `chat.message`, or `chat.turn_error`. While set, `ChatMessageList.vue` renders a lightweight animated indicator (e.g. `NSpin` or a pulsing "…" bubble) in the position the next Agent message will appear — not just for the very first moment after sending, but for any gap where no live content has arrived yet, since a multi-roundtrip turn can have silent gaps between an intermediate tool call finishing and the next one's `chat.tool_progress` firing.

This is a supplement to the existing `chat.tool_progress` labels ("Lese Seite…" etc., spec 00-03), not a replacement — those already cover the gap *between* tool calls once the turn is visibly moving; this covers the gap *before* the turn shows any activity at all.

---

## Unicode Escape Sequences Displayed Literally

**Symptom** (reported, not yet reproduced): a Confirm-Card showed a criterion's name/description as literal `Grundstücksgröße` text instead of "Grundstücksgröße" — JSON `\uXXXX` escape sequences rendered as-is rather than decoded.

**Investigation so far**: a fresh live `propose_criteria` call with umlauts in both name and description, tested end-to-end immediately after this was reported, came back clean at every layer checked — raw HTTP response bytes (`curl` + `grep`), and JSON-parsed via Python — real UTF-8 `ü`/`ö`/`ß` throughout, no escaping anywhere. Could not reproduce.

Background for whoever picks this up: `\uXXXX` is valid JSON either way (Claude's own JSON generation for tool-call arguments isn't guaranteed to pick the same escaping style on every response — some turns may emit literal UTF-8 bytes, others may `\u`-escape non-ASCII characters; both decode identically). `agent.StoredToolCall.Input` is stored and relayed as `json.RawMessage` end-to-end — never re-encoded — so whichever style Claude used for a given response is what ends up in `chat_message.tool_calls`. That's expected and should be transparent to the frontend: a standard `JSON.parse` (which `fetch(...).json()` performs) decodes `\uXXXX` correctly regardless of nesting depth, so the escape style alone shouldn't explain a literal-text render.

**Open — needs a live repro to diagnose further**:
- Confirm whether this still happens on a *fresh* turn (today's build) or only appears in old, already-persisted messages from earlier in the day (possibly before an unrelated fix landed) — if only the latter, this may already be resolved and the report is stale data.
- If it reproduces fresh: capture the exact API response body for that message (`GET .../chat/sessions/{id}/messages`, look at the specific `tool_calls[].input` value) and check whether the escape sequence is single- or double-encoded (i.e. does the JSON contain `ü` as a normal escape, or literally `\\u00fc` / a JSON string *value* that itself contains the six characters `\`, `u`, `0`, `0`, `f`, `c`) — the latter would point to a double-marshal somewhere in the Go pipeline (`tools.go`'s `mustJSON`, `agent_service.go`'s `toolCallsJSON`/`humanizeToolResult`, or the claude provider's argument accumulation) and be straightforward to fix once located; the former would point somewhere in the frontend not going through `JSON.parse` for this specific value.

---

## Verification

- Turn-in-progress indicator: send a message, confirm the indicator appears immediately and disappears the moment either streaming text or a tool-progress label arrives; trigger a multi-roundtrip turn and confirm the indicator reappears during a gap between tool calls if one occurs.
- Unicode escapes: reproduce live per the open questions above before attempting a fix — this section stays open until reproduced.
