---
type: spec
created: 2026-07-28
title: Manual Review & Edit
project: property-finder-obs-style
spec: manual-review-and-edit
phase: 1
order: 7
category: listings
status: ready
updated: 2026-07-28
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - listings
  - editing
---

## Overview

The user must be able to trace and correct every field the Agent writes, outside the chat. This is the app's transparency guarantee. A conversational ingestion pipeline is only trustworthy if the user can always fall back to a plain form and see and fix exactly what is stored.

---

## Listing Detail View

This is a plain form view of every structured field: title, price, sizes, address, coordinates, and seller info including `seller_notes`. It also shows a rendered view of `document_md` and the Criteria grade grid. It also shows the Listing's own freeform Notes ([[01-11-listing-notes]]). These are different from `seller_notes`. `status` is in the page header, not in the field grid. The grade grid is above the form, not below `document_md`. See [[01-10-listing-views-overhaul]] for the full layout and the reasons (most visits check the evaluation results).

You can reach it in three ways: through a master-detail split next to the List View; as a standalone full-page route for deep links (chat inline card, bookmarks); and through the inline card in a chat turn (see [[01-01-chat-interface]], [[01-10-listing-views-overhaul]]).

---

## Editing Behaviour

- Editing is explicit. A "Bearbeiten" button (means: "edit") switches the form to an editable state. "Speichern" (means: "save") sends every field changed since Bearbeiten in one `PATCH` call. "Abbrechen" (means: "cancel") discards the draft. It does not save on blur for each field. See [[01-10-listing-views-overhaul]] for why this changed from the first save-immediately design. Easy per-field autosave felt unpredictable once the form had eighteen fields in several sections.
- Editing a field adds its name to `listing.user_confirmed_fields` ([[../00_infrastructure_setup/00-02-database-schema#listing]]). This is still worked out per field that really changed value. It now happens at Speichern time for the whole batch, not on each blur event.
- A later Agent Follow-Up Instruction may touch a field listed in `user_confirmed_fields`. The Agent then asks for confirmation before it overwrites the field ("Preis wurde von dir manuell auf 310.000 kr gesetzt — soll ich das überschreiben?", means: "You set the price to 310,000 kr by hand — should I overwrite that?"). It does not silently overwrite a manual correction. Fields the user never touched are updated freely.
- Editing any structured field that is used in Criteria evaluation sets `listing.needs_reevaluation = true`. The user starts re-evaluation by hand, with the button in the detail view or the banner in the list view ([[../../docs/adr/0015-manual-bulk-reevaluation]]).
- Editing `document_md` directly is also supported (a plain Markdown textarea). It does not start re-evaluation by itself, because it is prose and not a structured field.

---

## Status Management

Unchanged from the predecessor project: `new`, `waiting`, `in_negotiation`, `visited`, `rejected`, `archived`. `rejected` requires a comment. You can set the Status here and also in the chat ("das ist raus, zu teuer für die Fläche" means: "that is out, too expensive for the size"). In that case the Agent asks you to confirm setting Status to `rejected` with that text as the comment.

---

## Open Questions / TBD

- Should `user_confirmed_fields` ever clear by itself (for example after N months)? Or should it stay until the user removes it? The current lean is permanent, because a manual correction rarely becomes wrong again on its own.

---

## Creating a Listing by Hand and the Duplicate Warning

The "Inserat anlegen" button in the Listing panel opens a small form (`ListingCreateModal.vue`). It sends `POST /api/profiles/{profileId}/listings`. The unique index on `source_url` only covers URL imports, so a hand-made listing needs its own check (issue #10).

- Before the insert, the backend (`ListingService.Create`, `findDuplicates`) compares the new listing with the profile's existing listings. Two listings match on **address** when the addresses are equal after lower-casing and collapsing whitespace. They match on **coordinates** when latitude and longitude are both set and each differs by at most 0.00001 degrees (about 1 m). An empty address never matches.
- On a match, the backend does not insert. It answers `409` with `{ "error": "...", "code": "duplicate_listing", "duplicates": [{ "id", "title", "address", "matches": ["address" | "coordinates"] }] }`.
- This is a warning, not a block. The frontend shows the matching listings in a warning box and offers "Trotzdem anlegen" (means: "create anyway"). That repeats the request with `?confirm_duplicate=true`, which skips the check.
- Only the REST create path checks. Agent `save_listing` has its own `source_url` deduplication ([[01-03-listing-ingestion-via-chat#Deduplication]]) and is not changed.
