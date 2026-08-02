---
type: spec
created: 2026-07-28
title: Chat Interface (per Search Profile)
project: property-finder-obs-style
spec: chat-interface
phase: 1
order: 1
category: chat
status: ready
updated: 2026-07-28
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - chat
---

## Overview

The Chat is the primary way the user works with a Search Profile. It sits alongside — not instead of — the List, Ranking, and Map views: opening a profile shows the Chat by default, with a persistent side panel or tab strip to jump to the other views. Actions the Agent takes (new Listing saved, Listing updated, Context change proposed) are reflected immediately in those other views without a page reload.

---

## Layout

- Message list: user turns, Agent turns, and inline tool-call progress ("Lese Seite…", "Folge Makler-Link…", "Extrahiere Bilder (3)…", "Speichere Inserat…")
- Input: text box + file drop zone (PDF, images) + paste-URL shortcut (pasting a bare URL auto-suggests "Dieses Inserat auswerten?")
- When an Agent turn creates or updates a Listing, the reply includes an inline card (title, price, thumbnail, Score) linking to the Listing detail view — backed by `chat_message.listing_id` ([[../00_infrastructure_setup/00-02-database-schema#chat_message]])
- **Token usage donut chart**: in the chat header, a small donut chart shows `sessionTokenCount / 200000` as a percentage. Color-coded: green (0–60%), yellow (60–80%), red (80–100%). Purely informational — no automatic action.
- **„Neue Session“ button**: in the chat header. Creates a new `chat_session` row. Old session becomes read-only (messages still visible), new session starts with empty context. Confirmation dialog warns if open Confirm-Cards exist: „Eine offene Bestätigung geht verloren. Trotzdem neue Session starten?" Disabled while a turn is in flight. Old sessions viewable via dropdown. Backend: `POST /api/profiles/{id}/chat/sessions`. See [[../../docs/adr/0014-context-window-token-display-manual-reset]].

---

## Behaviour

- Every message is persisted (`chat_message`); reopening a profile resumes the same session, scrolled to the latest turn
- Streaming: Agent text and tool-call progress arrive incrementally over SSE (see [[../00_infrastructure_setup/00-01-tech-stack#Chat Streaming]]); the input is disabled while a turn is in flight, with a visible "stop" control that cancels the in-flight Agent loop. Since the loop is fire-and-forget ([[../../docs/adr/0008-agent-turn-fire-and-forget]]), "stop" sets a cancellation flag; any tool call currently executing runs to completion, but no further LLM requests are made. Already-persisted results (a `save_listing` that completed) are kept — the turn is aborted mid-flight, not rolled back.
- When a user message has no follow-up Agent message (server restarted mid-turn, or "stop" was used before any reply), the UI shows "Antwort fehlt — erneut senden?" as a small button below the user message, re-submitting the same prompt
- Uploading a PDF and sending no text defaults to "auswerten wie ein neues Inserat"
- The user can scroll back through history and click any past inline Listing card to jump to its detail view

---

## Not in Scope for MVP

- Multi-user / concurrent sessions on one profile (single-user app, not needed)
- Editing or deleting past chat messages
- Branching/forking a conversation
