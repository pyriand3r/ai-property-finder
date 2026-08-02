---
type: adr
title: "Context-window management: token display + manual new-session button, no automatic truncation"
project: property-finder-obs-style
status: accepted
created: 2026-08-02
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/01_mvp/01-01-chat-interface]].

## Context

Every Agent turn sends the full conversation history plus Global Context plus Profile Context to the LLM. As a profile accumulates turns across multiple listings, the combined token count grows. Claude's context window is 200K tokens — enough for many turns, but not infinite.

The traditional solutions are sliding windows (truncate oldest messages), automatic summarisation (compress old turns into a summary paragraph), or per-entity sessions (one chat per listing). All add backend complexity: truncation logic, summary generation, session routing. For a single-user app where the user is present and engaged in every turn, none of this complexity is justified — the user can see the problem coming and act on it.

## Decision

**No automatic context-window management. The user sees token usage and starts a fresh session when needed.**

Three components:

### 1. Token Counter (Backend)

`context_builder.go` tracks cumulative token usage across turns within a session. After each `Provider.Chat()` call, it reads the actual token counts from the API response (`usage.input_tokens` + `usage.output_tokens` — every provider returns these). No estimation, no client-side tokenizer.

```go
sessionTokenCount += usage.InputTokens + usage.OutputTokens
```

The count is stored in-memory per active Agent loop, not persisted — it resets on server restart (acceptable: the display recovers on the next turn when fresh usage data arrives). For display purposes, the last known count is attached to `chat_session` as a cached column (`token_count_estimate INTEGER`) so the frontend can show it on page load without an active turn.

### 2. Donut Chart (Frontend)

In the chat header, a small donut chart shows `sessionTokenCount / 200000` as a percentage:

| Range | Color | Meaning |
|---|---|---|
| 0–60% | Green | Plenty of room |
| 60–80% | Yellow | Getting full |
| 80–100% | Red | Near limit |

Purely informational. No automatic action. The user decides when to start fresh.

### 3. New Session Button (Frontend + Backend)

A button in the chat header: **„Neue Session“**. On click:

1. Backend creates a new `chat_session` row for the profile (same `profile_id`, new UUID)
2. The old session is marked as read-only — its messages remain visible in the chat history UI, but new messages go into the new session
3. Frontend switches the active session and clears the donut chart
4. Confirmation dialog: „Neue Session starten? Der bisherige Verlauf bleibt als Chat-Verlauf erhalten, aber die KI startet mit leerem Kontext.“

The Agent's system prompt for the new session includes exactly the same Global + Profile Context as before. The conversation history is empty (first turn). The Agent does not know what happened in previous sessions — the user tells it what's relevant.

Backend endpoint: `POST /api/profiles/{id}/chat/sessions` — creates a new `chat_session` row, returns its ID. The frontend switches to it.

### Cross-Listing Behaviour

The Agent's context contains: the current session's messages + Global Context + Profile Context. It does **not** automatically have other listings' details. When the user asks "vergleiche mit Kappmuren 2", the Agent:

1. Calls `list_profile_listings` to find the listing
2. Loads its `document_md` via a tool (existing or new `get_listing` tool)
3. Compares and answers

The user can also paste a listing URL mid-session to ingest a new listing into the current session — the Agent follows the normal ingestion flow. The session is not locked to one listing.

### PDF Handling (Clarification)

Uploaded PDFs are sent as native document content blocks per [[0005-pdf-native-document-input]]. After the ingestion turn completes, the PDF file is deleted from disk — it is **not** resent in subsequent turns. The extracted data lives in `listing` + `chat_message` rows. This prevents PDFs from silently consuming large portions of the context window across turns.

## Alternatives considered

- **Sliding window** (truncate oldest N messages) — rejected: requires deciding what to cut (tool-call pairs must stay together, important context must stay). Error-prone and fragile.
- **Automatic summarisation** (compress old turns into a summary) — rejected: adds an LLM call to every N-th turn just for maintenance. Overkill for a single-user app.
- **Per-listing sessions** (one `chat_session` per listing) — rejected: forces the user to switch sessions to compare listings. Cross-listing questions are common in the reference workflow.
- **No management at all, crash on overflow** — rejected: a hard crash is a bad user experience. The donut chart gives visibility; the new-session button gives control.

## Consequences

- `chat_session` gains one column: `token_count_estimate INTEGER` — updated after each turn, cached for frontend display on page load
- `context_builder.go` gains ~10 lines for token accumulation
- Frontend gains one donut-chart component and one button — ~30 lines of Vue
- Backend gains one endpoint: `POST /api/profiles/{id}/chat/sessions` → returns new session ID
- `POST /api/profiles/{id}/chat/sessions/{sid}/messages` — chat messages are now scoped to a session ID in the URL (minor route change)
- The 🟡 "Chat-History-Strategie im Context Window" task is resolved — no sliding window, no summarisation, no per-listing session routing
