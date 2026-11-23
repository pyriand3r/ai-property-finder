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
updated: 2026-11-23
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - chat
  - context
---

## Overview

Two Markdown text stores guide how the Agent behaves across turns. See [[../CONTEXT#Context File]]. This spec explains how they are seeded and edited. It also explains how the user confirms changes that the Agent proposes.

---

## Global Context

There is one row for the whole app (`context.scope = 'global'`). The first migration seeds it with a default text. The default covers the habits seen in the reference Obsidian workflow:

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

The user can edit it at any time in `ContextEditorView.vue`. The Agent may propose additions. For example, it may see the user ask for the same thing again and again across several profiles. It uses the same confirmation flow as the Profile Context, described below.

---

## Profile Context

There is one row per Search Profile (`context.scope = 'profile'`). It is empty by default. It grows from two sources:

1. **The user writes directly** in the editor. For example, the user pastes the exact "Makler-Link immer folgen" instruction (means: "always follow the agent link") from the reference Obsidian workflow.
2. **The Agent proposes** a change after the user corrects it in conversation. For example, the user says "bei Booli-Inseraten immer dem 'Läs mer hos mäklaren'-Link folgen, die Beschreibung dort ist besser" (means: "for Booli listings, always follow the 'Läs mer hos mäklaren' link, the description there is better"). The Agent's `update_context` tool call turns this into a lasting instruction.

---

## Confirmation

The Agent's Context changes are **never applied silently**. The `update_context` tool call ([[../00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]]) checks the proposal. It returns `{"status": "pending_confirmation", ...}` and does not write to `context.content_md`. The full proposal is stored in the `chat_message.tool_calls` JSONB. There is no separate staging table. The chat turn shows a diff card (current text vs. proposed change). The card has two buttons: **Übernehmen** (means: "Accept") and **Verwerfen** (means: "Discard").

On **Übernehmen**, the frontend calls `POST /api/chat/messages/{msg_id}/tool-calls/{call_idx}/confirm`. The backend reads the tool-call arguments from `chat_message`. It appends them to the **current** `context.content_md`. It reads this text fresh at confirm time, not as it was when the Agent proposed the change. It then creates a `context_revision` row with `changed_by = 'agent'`, linked to the confirming `chat_message`. Append is the only supported operation. So there is no check for stale base text. Appending to the current text can never lose data. A replace operation could lose data.

On **Verwerfen**, the frontend calls `POST .../reject`. No state changes.

Both endpoints are idempotent. Once a call is confirmed or rejected, acting on it again returns `409 Conflict`. It does not silently apply it again, and it does not silently do nothing. The Übernehmen/Verwerfen buttons on the Confirm-Card show this saved status. If the user reopens the Chat tab or reloads the page, the card shows its real resolved state. It does not go back to the pending button pair ([[../../docs/adr/0017-tool-call-status-in-jsonb]]).

Direct user edits in the editor are written at once, with no confirmation. They create a `context_revision` row with `changed_by = 'user'`.

Full rationale: [[../../docs/adr/0009-proposal-pending-in-tool-calls]], [[../../docs/adr/0017-tool-call-status-in-jsonb]].

---

## Editor

`ContextEditor.vue` (Layer 7.2) is shared by both scopes through `useContextEditor.ts`. This composable takes the four operations (fetch/update/history/restore) as injected functions. This avoids copying the load, save, and error state for each scope.

- View mode shows `content_md` as Markdown (`marked` + `DOMPurify`, the same pipeline as the Listing Document preview in `ListingDocument.vue`). **Bearbeiten** (means: "Edit") switches to a raw-Markdown `NInput` textarea. It has explicit **Speichern** (means: "Save") and **Abbrechen** (means: "Cancel") buttons. There is no preview *toggle* while editing. This follows the Bearbeiten/Speichern/Abbrechen pattern first used for `ListingForm.vue` ([[01-10-listing-views-overhaul]]). It avoids a three-state edit/preview/view switch.
- The history panel lists `context_revision` rows, newest first. Each row shows a **Nutzer** (means: "User") or **Agent** tag (`changed_by`), the timestamp, and a **Wiederherstellen** (means: "Restore") action. Wiederherstellen asks for confirmation first. It uses the same `useDialog()` pattern as "Neue Session starten?" in `ChatView.vue`. It asks because it overwrites the current text. But nothing is really lost. Restore works as a normal edit whose new text comes from the history. So the text from before the restore is saved as a new revision, like any other change.
- `POST /api/context/global/revisions/{revisionId}/restore` and `POST /api/profiles/{profileId}/context/revisions/{revisionId}/restore`: the backend checks that the revision's `context_id` matches the target Context before it applies it. If not, it returns `400`. So a revision id from one Context (for example, another profile's history) cannot be replayed onto a different Context.
- The user reaches the Profile Context editor from the profile's Chat view (`ProfileContextPanel.vue` in the "Context" tab, `ProfileShellView.vue`). The user reaches the Global Context editor (`GlobalContextView.vue`, route `/context`) through a "Globaler Kontext" link (means: "Global context") on the profile list page. There is no dedicated Settings shell yet. So this is a practical entry point near Settings for now. It is not nested under any one profile.

### Portal Rules editor

`PortalRulesEditor.vue` is part of both Context scopes. It is hidden while the raw Markdown editor is active so one view cannot overwrite the other's unsaved draft. It lists Portal Rules by domain. **Regel hinzufügen** (means: "Add rule") and **Bearbeiten** (means: "Edit") open a small form for the domain and its natural-language instruction. Saving emits the complete changed Markdown through the same `PUT` call as a raw Context edit. There is no Portal Rule database table or API resource. This is only a structured view of Context text, as required by ADR 0003 and `spec.md` Phase 2.

The structured editor uses this Markdown convention:

```markdown
## Portal Rules

### booli.se
Folge immer dem Link zur Makler-Seite.

### hemnet.se
Lies die vollständige Beschreibung und lade auch den Grundriss.
```

The `## Portal Rules` heading starts the section. Each `### domain` heading starts one rule. A following level-1 or level-2 heading ends the section. Level-3 headings inside rule text are not possible because they would start another domain. The form therefore rejects level-1 to level-3 ATX headings in rule text. Level-4 to level-6 headings and other Markdown are allowed.

Domain names are saved in lowercase and must be unique without regard to case. Existing rules can be renamed or edited. Adding or editing a rule preserves Markdown before and after the changed rule. The serializer keeps the current CRLF or LF line-ending style. Other spellings such as `## Portalregeln` remain ordinary raw Markdown and do not appear in the structured list. The editor manages the first canonical `## Portal Rules` section only. A second section with the same name remains raw text.

---

## Open Questions / TBD

- Should Profile Context support variables like `{{profile_name}}`? Or should it stay plain free text, as it is today? The current lean is free text, because the reference Obsidian workflow never needed variables.
- Is a size warning needed once a Profile Context grows large enough to use a real share of the token budget on every turn?
