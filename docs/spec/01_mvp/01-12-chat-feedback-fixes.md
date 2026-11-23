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

We found two more issues while live-testing the fixes from [[01-09-chat-reliability-fixes]]. Neither is a data-correctness bug like those in 01-09. Both are gaps in chat-UI feedback. They make a working Agent turn *look* broken.

---

## Turn-In-Progress Indicator

**Symptom**: the user sends a message (including a "weitermachen" continuation, meaning "continue"). Nothing visible happens until the first `chat.text_delta`/`chat.tool_progress` SSE event arrives. A turn may start with a slow tool call before any text. Then it looks like "nothing happened". The "weitermachen" fix in 01-09 tried to remove the *cause* of this confusion. But the symptom (silence) can still happen in a healthy turn that just needs a few seconds to start.

**Fix**: `useChat.ts` gets a state flag. It is set the moment the `POST` of `send()` succeeds. At that point the turn has been started on the server (fire-and-forget, per ADR 0008). It is cleared the moment the *first* live signal arrives for that turn. That signal can be a `chat.text_delta`, a `chat.tool_progress`, the final `chat.message`, or `chat.turn_error`. While the flag is set, `ChatMessageList.vue` shows a light animated indicator (for example `NSpin` or a pulsing "…" bubble). It shows it at the place where the next Agent message will appear. This applies not only right after sending. It applies to any gap where no live content has arrived yet. A turn with several roundtrips can have silent gaps. One example is the time between an intermediate tool call finishing and the `chat.tool_progress` of the next one.

This adds to the existing `chat.tool_progress` labels ("Lese Seite…" etc., spec 00-03). It does not replace them. Those labels already cover the gap *between* tool calls once the turn visibly moves. The new indicator covers the gap *before* the turn shows any activity. ("Lese Seite…" means "reading page…".)

---

## Unicode Escape Sequences Displayed Literally

**Symptom** (reported, not yet reproduced): a Confirm-Card showed the name/description of a criterion as literal `Grundstücksgröße` text instead of "Grundstücksgröße". JSON `\uXXXX` escape sequences were shown as-is and not decoded.

**Investigation so far**: right after the report, we tested a fresh live `propose_criteria` call end-to-end. It had umlauts in both name and description. The result was clean at every layer we checked. We checked the raw HTTP response bytes (`curl` + `grep`) and the JSON parsed with Python. Both had real UTF-8 `ü`/`ö`/`ß` everywhere and no escaping. We could not reproduce the problem.

Background for whoever picks this up: `\uXXXX` is valid JSON either way. Claude's own JSON generation for tool-call arguments does not always pick the same escaping style. Some turns may emit literal UTF-8 bytes. Others may `\u`-escape non-ASCII characters. Both decode the same way. `agent.StoredToolCall.Input` is stored and passed on as `json.RawMessage` from start to end. It is never re-encoded. So whatever style Claude used for a response is what ends up in `chat_message.tool_calls`. This is expected. The frontend should not notice it. A standard `JSON.parse` (which `fetch(...).json()` runs) decodes `\uXXXX` correctly at any nesting depth. So the escape style alone should not explain literal text in the UI.

**Re-tested 2026-08-07** (after the turn-in-progress indicator fix above): we made a fresh `propose_criteria` call with `Grundstücksgröße` in both name and description. We checked the raw `GET .../messages` response body with `grep`. It had real UTF-8 throughout and no `\uXXXX`. This is the second clean result in a row. We still cannot reproduce it on current `main`. We leave this open and do not close it. A report that does not reproduce is not a confirmed fix. It just does not happen right now. But with two independent clean checks on record, it is less and less likely to be a live bug. It is more likely stale or misremembered data from before the 01-09 fixes landed.

**Fix (issue #6)**: still not reproduced live, so the fix is defensive and frontend-only. `ChatConfirmCard.vue` decodes literal `\uXXXX` sequences in the displayed name, description and context content with `decodeUnicodeEscapes` (`src/utils/chat.ts`). A normal `JSON.parse` already decodes proper escapes. The only way the UI can still show `\uXXXX` is a double-escaped string value, and this covers that case. Limit: the stored `tool_calls[].input` is not changed. Confirming applies the stored value as is (`ToolCallService`). If a double-escaped value is ever seen live, normalise it in the backend too.

---

## Verification

- Turn-in-progress indicator: send a message. Confirm the indicator appears at once. Confirm it disappears the moment streaming text or a tool-progress label arrives. Trigger a turn with several roundtrips. Confirm the indicator comes back during a gap between tool calls, if one occurs.
- Unicode escapes: not reproducible live. Check the fix with a Confirm-Card whose stored input contains the six characters `\u00fc` in a string value. The card should show `ü`.
