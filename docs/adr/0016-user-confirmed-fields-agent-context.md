---
type: adr
title: "user_confirmed_fields: Agent sees them in context, save_listing skips unless force_overwrite"
project: property-finder-obs-style
status: accepted
created: 2026-08-02
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/01_mvp/01-07-manual-review-and-edit]].

## Context

The user can manually edit any Agent-written field in the listing detail form. These edits must survive subsequent Agent turns — the Agent shouldn't silently overwrite a manual correction just because it re-reads the source page and sees a different value. The spec states: "a subsequent Agent Follow-Up Instruction that touches a `user_confirmed_fields` entry asks for confirmation before overwriting it."

The question: how does the Agent *know* which fields are confirmed, and how does the confirmation flow actually work?

## Decision

### 1. Agent sees confirmed fields in every turn

`context_builder.go` appends a block after the system preamble and context files, for any listing that has non-empty `user_confirmed_fields`:

```
## Aktuelles Inserat (Kappmuren 2)
Manuell bestätigte Felder — nur nach expliziter Zustimmung überschreiben:
- price: 310.000 kr
- address: Kappmuren 12, 240 32 Malmö
```

This is plain text, same format as the Context Files, assembled at prompt-build time. The Agent reads it the same way it reads Portal Rules — natural language, no structured schema. The preamble already contains the rule: "user_confirmed_fields: Felder die der Nutzer manuell geändert hat, werden dir mitgeteilt. Überschreibe sie nur nach Rückfrage." ([[00-04-system-prompt-preamble]]).

### 2. `save_listing` has a force_overwrite parameter

```json
// Normal call — confirmed fields are protected
save_listing({
  "listing_id": "abc-123",
  "fields": {
    "price": 305000,
    "size_land": 1200
  }
})

// Response if price is in user_confirmed_fields:
{
  "status": "ok",
  "listing_id": "abc-123",
  "skipped_fields": ["price"],
  "note": "price wurde nicht überschrieben — Feld ist manuell bestätigt."
}

// Force call — user said "überschreib"
save_listing({
  "listing_id": "abc-123",
  "fields": {
    "price": 305000
  },
  "force_overwrite": ["price"]
})

// Response:
{
  "status": "ok",
  "listing_id": "abc-123",
  "overwritten_fields": ["price"]
}
```

The backend logic in `save_listing`:

```go
for field, value := range fields {
    if slices.Contains(userConfirmedFields, field) && !slices.Contains(forceOverwrite, field) {
        skipped = append(skipped, field)
        continue  // don't write this field
    }
    // write field normally
}
if len(skipped) > 0 {
    response.SkippedFields = skipped
}
```

When a field is forcefully overwritten, it is **removed** from `user_confirmed_fields` — the user's correction is replaced by the Agent's new value. If the user wants to lock it again, they edit it manually in the detail form again.

### 3. Agent flow for overwriting

```
Agent:  [calls save_listing with new data from source page]
        Antwort: "price wurde nicht überschrieben — Feld ist manuell bestätigt."

Agent:  [im Chat] Auf der Maklerseite steht 305.000 kr.
        Dein manuell gesetzter Preis (310.000) wurde nicht überschrieben.

User:   Überschreib den Preis.

Agent:  [calls save_listing with force_overwrite: ["price"]]
        Preis auf 305.000 kr aktualisiert.
```

No confirm-card UI, no pending-proposal mechanism, no separate endpoint. The confirmation is conversational — the user says "yes" in chat, the Agent re-calls the tool with `force_overwrite`. This reuses the exact same `save_listing` tool, same Agent loop, same SSE streaming. The only new thing is one optional parameter on an existing tool.

### 4. Manual edit → confirmed field

When the user manually edits a field in the detail form (`01-07`), the backend:

```go
// Append field name to user_confirmed_fields (if not already there)
UPDATE listing SET 
    user_confirmed_fields = array_append_unique(user_confirmed_fields, 'price'),
    price = $1
WHERE id = $2;
```

`array_append_unique` is a helper that only adds if the value is not already in the array. PostgreSQL doesn't have this natively, so it's either a small Go helper or a `CASE WHEN NOT (value = ANY(array)) THEN array_append(...)` expression.

## Why No Confirm-Card

This is different from `propose_criteria` and `update_context` (which use confirm-cards per [[0009-proposal-pending-in-tool-calls]]). Those change *global* state (profile-wide Criteria, app-wide Context). A field overwrite is a *per-listing* operation with a clear scope and a single value — the user said "überschreib den Preis", the Agent does it, done. No diff to review, no multi-step application. The conversational confirmation is the right granularity.

## Consequences

- `context_builder.go` appends a confirmed-fields block when a listing is in context (~5 lines)
- `save_listing` gains one optional parameter `force_overwrite: string[]` — validates field names exist, applies skip logic (~10 lines)
- `save_listing` tool schema updated: `force_overwrite` as optional array of field name strings
- Preamble rule already handles the Agent's behaviour, no change needed
- No new endpoints, no new UI components, no new tables
- Manual edit handler adds field name to `user_confirmed_fields` array (~3 lines)
