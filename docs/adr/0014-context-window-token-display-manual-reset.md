---
type: adr
title: "Context window: show token use and a manual new-session button, no automatic truncation"
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

Every Agent turn sends the full conversation history, the Global Context and the Profile Context to the LLM. A profile collects more turns as the user works on more listings. So the total token count keeps growing. Claude's context window is 200K tokens. That is enough for many turns, but it is not infinite.

The usual solutions are:

- sliding windows (cut the oldest messages)
- automatic summaries (squeeze old turns into one summary paragraph)
- one session per entity (one chat per listing)

All of them add backend work: cut logic, summary generation, session routing. This app has one user, and that user is present in every turn. The user can see the problem coming and act on it. So this extra work is not justified.

## Decision

**No automatic context-window management. The user sees token usage and starts a fresh session when needed.**

There are three parts:

### 1. Token Counter (Backend)

`context_builder.go` tracks the total token usage across the turns of a session. After each `Provider.Chat()` call, it reads the real token counts from the API response (`usage.input_tokens` + `usage.output_tokens` — every provider returns these). There is no estimation and no client-side tokenizer.

```go
sessionTokenCount += usage.InputTokens + usage.OutputTokens
```

The count lives in memory for each active Agent loop. It is not persisted, so it resets when the server restarts. This is acceptable: the display recovers on the next turn, when new usage data arrives. For display, the last known count is saved on `chat_session` in a cached column (`token_count_estimate INTEGER`). This way the frontend can show it on page load, even when no turn is running.

### 2. Donut Chart (Frontend)

In the chat header, a small donut chart shows `sessionTokenCount / 200000` as a percentage:

| Range | Color | Meaning |
|---|---|---|
| 0–60% | Green | Plenty of room |
| 60–80% | Yellow | Getting full |
| 80–100% | Red | Near limit |

It is for information only. It triggers no automatic action. The user decides when to start fresh.

### 3. New Session Button (Frontend + Backend)

The chat header has a button: **„Neue Session“** (means: "New session"). When the user clicks it:

1. The backend creates a new `chat_session` row for the profile (same `profile_id`, new UUID).
2. The old session becomes read-only. Its messages stay visible in the chat history UI, but new messages go into the new session.
3. The frontend switches to the new session and clears the donut chart.
4. A confirmation dialog appears: „Neue Session starten? Der bisherige Verlauf bleibt als Chat-Verlauf erhalten, aber die KI startet mit leerem Kontext.“ (means: "Start a new session? The earlier history stays as chat history, but the AI starts with an empty context.")

The Agent's system prompt for the new session has exactly the same Global Context and Profile Context as before. The conversation history is empty (first turn). The Agent does not know what happened in earlier sessions. The user tells it what matters.

Backend endpoint: `POST /api/profiles/{id}/chat/sessions` — creates a new `chat_session` row and returns its ID. The frontend switches to it.

### Cross-Listing Behaviour

The Agent's context has the messages of the current session, the Global Context and the Profile Context. It does **not** automatically have the details of other listings. When the user asks "vergleiche mit Kappmuren 2" (means: "compare with Kappmuren 2"), the Agent:

1. Calls `list_profile_listings` to find the listing.
2. Loads its `document_md` with a tool (an existing or new `get_listing` tool).
3. Compares and answers.

The user can also paste a listing URL in the middle of a session. This adds a new listing to the current session, and the Agent follows the normal ingestion flow. The session is not locked to one listing.

### PDF Handling (Clarification)

Uploaded PDFs are sent as native document content blocks, as described in [[0005-pdf-native-document-input]]. After the ingestion turn ends, the PDF file is deleted from disk. It is **not** sent again in later turns. The extracted data lives in `listing` and `chat_message` rows. This stops PDFs from quietly using up a large part of the context window across turns.

## Alternatives considered

- **Sliding window** (cut the oldest N messages) — rejected: someone must decide what to cut. Tool-call pairs must stay together, and important context must stay. This is error-prone and fragile.
- **Automatic summarisation** (squeeze old turns into a summary) — rejected: it adds an LLM call to every N-th turn just for upkeep. This is too much for a single-user app.
- **Per-listing sessions** (one `chat_session` per listing) — rejected: the user would have to switch sessions to compare listings. Questions across listings are common in the reference workflow.
- **No management at all, crash on overflow** — rejected: a hard crash is a bad user experience. The donut chart gives visibility. The new-session button gives control.

## Consequences

- `chat_session` gets one new column: `token_count_estimate INTEGER`. It is updated after each turn and cached so the frontend can show it on page load.
- `context_builder.go` gets about 10 lines for token counting.
- The frontend gets one donut-chart component and one button — about 30 lines of Vue.
- The backend gets one endpoint: `POST /api/profiles/{id}/chat/sessions` → returns the new session ID.
- `POST /api/profiles/{id}/chat/sessions/{sid}/messages` — chat messages are now tied to a session ID in the URL (a small route change).
- The 🟡 task "Chat-History-Strategie im Context Window" (means: "chat history strategy in the context window") is resolved. There is no sliding window, no summarisation and no per-listing session routing.
