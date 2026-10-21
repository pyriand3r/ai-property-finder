---
type: adr
title: "user_confirmed_fields: the Agent sees them in context, and save_listing skips them unless force_overwrite is set"
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

The user can edit any Agent-written field by hand in the listing detail form. These edits must survive later Agent turns. The Agent must not quietly overwrite a manual correction just because it reads the source page again and sees a different value. The spec says: "a subsequent Agent Follow-Up Instruction that touches a `user_confirmed_fields` entry asks for confirmation before overwriting it."

The question: how does the Agent *know* which fields are confirmed, and how does the confirmation flow work?

## Decision

### 1. Agent sees confirmed fields in every turn

`context_builder.go` adds a block after the system preamble and the context files. It does this for any listing that has a non-empty `user_confirmed_fields`:

```
## Aktuelles Inserat (Kappmuren 2)
Manuell bestätigte Felder — nur nach expliziter Zustimmung überschreiben:
- price: 310.000 kr
- address: Kappmuren 12, 240 32 Malmö
```

(The block text means: "Current listing (Kappmuren 2). Manually confirmed fields — only overwrite after explicit consent.")

This is plain text, in the same format as the Context Files. It is built when the prompt is built. The Agent reads it the same way it reads Portal Rules: as natural language, with no structured schema. The preamble already has the rule: "user_confirmed_fields: Felder die der Nutzer manuell geändert hat, werden dir mitgeteilt. Überschreibe sie nur nach Rückfrage." (means: "fields the user changed by hand are told to you; overwrite them only after asking") ([[00-04-system-prompt-preamble]]).

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

The note means: "price was not overwritten — field is manually confirmed." The word "überschreib" means "overwrite".

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

When a field is overwritten with force, it is **removed** from `user_confirmed_fields`. The Agent's new value replaces the user's correction. To lock the field again, the user edits it by hand in the detail form again.

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

(In this example the Agent says in chat: "The broker's page shows 305.000 kr. Your manually set price (310.000) was not overwritten." The user says: "Overwrite the price." The Agent then says: "Price updated to 305.000 kr.")

There is no confirm-card UI, no pending-proposal mechanism and no separate endpoint. The confirmation happens in conversation. The user says "yes" in chat, and the Agent calls the tool again with `force_overwrite`. This reuses the same `save_listing` tool, the same Agent loop and the same SSE streaming. The only new thing is one optional parameter on an existing tool.

### 4. Manual edit → confirmed field

When the user edits a field by hand in the detail form (`01-07`), the backend does this:

```go
// Append field name to user_confirmed_fields (if not already there)
UPDATE listing SET 
    user_confirmed_fields = array_append_unique(user_confirmed_fields, 'price'),
    price = $1
WHERE id = $2;
```

`array_append_unique` is a helper that adds a value only if it is not already in the array. PostgreSQL does not have this built in. So it is either a small Go helper or a `CASE WHEN NOT (value = ANY(array)) THEN array_append(...)` expression.

## Why No Confirm-Card

This differs from `propose_criteria` and `update_context`. Those use confirm-cards, as described in [[0009-proposal-pending-in-tool-calls]]. They change *global* state (profile-wide Criteria, app-wide Context). A field overwrite is a *per-listing* operation. It has a clear scope and a single value. The user said "überschreib den Preis" (means: "overwrite the price"), the Agent does it, and that is all. There is no diff to review and no multi-step apply. Confirming in conversation is the right level of detail.

## Consequences

- `context_builder.go` adds a confirmed-fields block when a listing is in context (~5 lines).
- `save_listing` gets one optional parameter `force_overwrite: string[]`. It checks that the field names exist and applies the skip logic (~10 lines).
- The `save_listing` tool schema is updated: `force_overwrite` is an optional array of field name strings.
- The preamble rule already covers the Agent's behaviour, so it needs no change.
- No new endpoints, no new UI components, no new tables.
- The manual edit handler adds the field name to the `user_confirmed_fields` array (~3 lines).
