---
type: spec
created: 2026-07-28
title: Global & Profile Context Files
project: property-finder-obs-style
spec: context-files
phase: 1
order: 2
category: chat
status: ready
updated: 2026-07-28
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - chat
  - context
---

## Overview

Two Markdown text stores steer the Agent's behaviour across turns — see [[../CONTEXT#Context File]]. This spec covers how they're seeded, edited, and how Agent-proposed changes get confirmed.

---

## Global Context

One row, app-wide (`context.scope = 'global'`). Seeded on first migration with a default covering the conventions observed in the reference Obsidian workflow ([[../../immobilienplan-schweden-portugal-oesterreich/schweden/00_readme#⚙️ Arbeitsanweisungen für zukünftige Sessions]]):

```markdown
## Bildauswertung
- Bilder wo möglich herunterladen und lokal speichern; ein Titelbild auswählen
- Kategorisieren: Außenansicht, Innenaufnahmen, Nebengebäude, Grundriss, Sonstiges
- Wenn keine Bilder ladbar sind (z. B. Platzhalter-only-Seiten): das im Chat sagen, nicht stillschweigend überspringen

## Listing Document
- Originaltext-Auszug in Ausgangssprache
- Vollständige deutsche Übersetzung
- Bewertungstabelle pro Kriterium
- Vor-/Nachteile im Vergleich zu bestehenden Inseraten im Profil

## Bewertung
- A–F pro Kriterium mit kurzer Begründung
- Score 0–100 wird automatisch aus Grades × Gewichtung berechnet
```

Editable by the user at any time via `ContextEditorView.vue`. The Agent may propose additions (e.g. after repeatedly seeing the user ask for the same thing across multiple profiles) — same confirmation flow as Profile Context, below.

---

## Profile Context

One row per Search Profile (`context.scope = 'profile'`), empty by default. Grows from two sources:

1. **User writes directly** in the editor — e.g. pastes in the exact "Makler-Link immer folgen" instruction from the reference workflow
2. **Agent proposes** after a conversational correction — e.g. user says "bei Booli-Inseraten immer dem 'Läs mer hos mäklaren'-Link folgen, die Beschreibung dort ist besser", and the Agent's `update_context` tool call turns that into a durable instruction

---

## Confirmation

Agent-proposed Context changes are **never applied silently**. The `update_context` tool call ([[../00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]]) validates the proposal and returns `{"status": "pending_confirmation", ...}` without writing to `context.content_md`. The full proposal lives in `chat_message.tool_calls` JSONB — no separate staging table. The chat turn renders a diff card (current text vs. proposed change) with **Übernehmen** / **Verwerfen** buttons.

On **Übernehmen**, the frontend calls `POST /api/chat/messages/{msg_id}/tool-calls/{call_idx}/confirm`. The backend reads the tool-call arguments from `chat_message`, validates (409 if the base text changed since the proposal), writes `context.content_md`, and creates a `context_revision` row with `changed_by = 'agent'`.

On **Verwerfen**, `POST .../reject` — no state change.

Direct user edits in the editor write immediately (no confirmation needed) and produce a `context_revision` row with `changed_by = 'user'`.

Full rationale: [[../../docs/adr/0009-proposal-pending-in-tool-calls]].

---

## Editor

- Plain Markdown textarea with preview toggle — no rich structure enforced, mirroring the freeform nature of the reference `AGENT.md`-style files
- History panel lists `context_revision` rows newest-first with a one-line diff summary and a **Wiederherstellen** action per row
- Profile Context editor is reachable from the profile's Chat view (tab); Global Context editor is reachable from a top-level Settings-adjacent entry point, not nested under any one profile

---

## Open Questions / TBD

- Whether Profile Context should support `{{profile_name}}`-style variable interpolation, or stay pure freeform text like today — leaning freeform, since the reference workflow never needed it
- Whether a size warning is needed once a Profile Context grows large enough to meaningfully eat into the token budget for every turn
