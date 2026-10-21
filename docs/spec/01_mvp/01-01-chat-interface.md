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

The Chat is the main way the user works with a Search Profile. It does not replace the List, Ranking, and Map views. It sits next to them. Opening a profile shows the Chat first. A side panel or tab strip stays visible, so the user can jump to the other views. When the Agent acts, the other views update at once, with no page reload. Examples of Agent actions: a new Listing is saved, a Listing is updated, or a Context change is proposed.

---

## Layout

- Message list: user turns, Agent turns, and inline tool-call progress ("Lese Seite…", "Folge Makler-Link…", "Extrahiere Bilder (3)…", "Speichere Inserat…"). These texts mean: "Reading page…", "Following agent link…", "Extracting images (3)…", "Saving listing…".
- Input: text box, file drop zone (PDF, images), and a paste-URL shortcut. If the user pastes a bare URL, the app suggests "Dieses Inserat auswerten?" (means: "Evaluate this listing?").
- When an Agent turn creates or updates a Listing, the reply includes an inline card. The card shows title, price, thumbnail, and Score. It links to the Listing detail view. It is backed by `chat_message.listing_id` ([[../00_infrastructure_setup/00-02-database-schema#chat_message]]).
- **Token usage donut chart**: the chat header shows a small donut chart. It shows `sessionTokenCount / 200000` as a percentage. It uses colors: green (0–60%), yellow (60–80%), red (80–100%). It is for information only. It triggers no automatic action. Make it big enough (ring size and stroke width) so the fill arc is easy to see at a glance. The first version was too small and too thick. The fill level was hard to tell apart from the track at a normal viewing distance.
- **„Neue Session“ button** (means: "New session"): it is in the chat header. It creates a new `chat_session` row. The old session becomes read-only, and its messages stay visible. The new session starts with an empty context. If Confirm-Cards are still open, a confirmation dialog warns the user: „Eine offene Bestätigung geht verloren. Trotzdem neue Session starten?" (means: "An open confirmation will be lost. Start a new session anyway?"). The button is disabled while a turn is in flight. The user can view old sessions in a dropdown. Backend: `POST /api/profiles/{id}/chat/sessions`. See [[../../docs/adr/0014-context-window-token-display-manual-reset]].

---

## Behaviour

- Every message is saved (`chat_message`). Reopening a profile resumes the same session, scrolled to the latest turn.
- Streaming: Agent text and tool-call progress arrive bit by bit over SSE (see [[../00_infrastructure_setup/00-01-tech-stack#Chat Streaming]]). The input is disabled while a turn is in flight. A visible "stop" control cancels the running Agent loop. The loop is fire-and-forget ([[../../docs/adr/0008-agent-turn-fire-and-forget]]). So "stop" sets a cancellation flag. A tool call that is running at that moment finishes. But no further LLM requests are made. Results that are already saved stay (for example, a `save_listing` that finished). The turn stops midway. It is not rolled back.
- Sometimes a user message has no Agent message after it. This happens if the server restarted during the turn, or if "stop" was used before any reply. Then the UI shows "Antwort fehlt — erneut senden?" (means: "Answer missing — send again?") as a small button below the user message. The button sends the same prompt again.
- If the user uploads a PDF and sends no text, the default text is "auswerten wie ein neues Inserat" (means: "evaluate like a new listing").
- The user can scroll back through the history. The user can click any past inline Listing card to jump to its detail view.

---

## Not in Scope for MVP

- Multi-user or concurrent sessions on one profile (the app has one user, so this is not needed)
- Editing or deleting past chat messages
- Branching or forking a conversation
