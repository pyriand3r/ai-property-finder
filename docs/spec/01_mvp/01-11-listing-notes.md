---
type: spec
created: 2026-08-05
title: Listing Notes
project: property-finder-obs-style
spec: listing-notes
phase: 1
order: 11
category: listings
status: ready
updated: 2026-08-05
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - listings
---

## Overview

A user review found a gap. The Detail View has no way to write freeform running notes about a Listing. Examples: "called the agent, viewing booked for Saturday" or "neighbor mentioned the roof was redone in 2019". This is **not** the same as `seller_notes`. That is a single field about the seller or contact person. It is part of the seller-info group ([[01-07-manual-review-and-edit]]). Listing Notes are the user's own notes about the property. The user can add as many as they want. Each note is its own object with a timestamp. Each can be edited and deleted on its own.

The `listing_note` table already exists in the schema. It was carried over unchanged from the predecessor project (see `backend/internal/db/migrations/000001_init.up.sql`). But no application code uses it yet. This spec builds the full stack.

---

## Data Model

`listing_note` has these columns today: `id, listing_id, content, created_at`. This spec adds `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()` with a new migration (`000003_listing_note_updated_at`). Every other editable table in this schema already has this column. Notes need it because they are edited in place, not deleted and recreated.

There is no `title` field. A note is a single content block, like the freeform per-listing notes in the reference Obsidian workflow.

---

## API

`internal/db/repository/note_repo.go` has the same shape as `image_repo.go` (`Create`, `GetByID`, `ListByListing`, `Delete`). It also has `Update` for edits in place:

```go
type NoteRepository interface {
    Create(ctx context.Context, note *domain.ListingNote) error
    ListByListing(ctx context.Context, listingID uuid.UUID) ([]domain.ListingNote, error)
    Update(ctx context.Context, id uuid.UUID, content string) error
    Delete(ctx context.Context, id uuid.UUID) error
}
```

`internal/service/note_service.go` is thin orchestration. It checks that the content is not empty. On create, it checks that the listing exists. It has the same shape as the simpler paths in `internal/service/image_service.go`.

`internal/api/notes.go` and its routes are nested under the listing. This matches the existing `/api/listings/{listingId}/images` pattern:

| Method | Path | |
|---|---|---|
| `GET` | `/api/listings/{listingId}/notes` | List, newest first |
| `POST` | `/api/listings/{listingId}/notes` | Create — body `{"content": "..."}` |
| `PATCH` | `/api/notes/{noteId}` | Update content |
| `DELETE` | `/api/notes/{noteId}` | Delete |

`listing_note.listing_id` already cascades when a listing is deleted (`ON DELETE CASCADE`, confirmed in the existing migration). `ListingService.Delete` needs no extra cleanup.

---

## Frontend

`ListingDetailPanel.vue` ([[01-10-listing-views-overhaul]]) gets a "Notizen" section (meaning "notes"). It is separate from the seller-info field group:

- The "Neue Notiz" button (meaning "new note") opens an empty inline card in edit mode.
- Each note shows as its own card. The card has the content text, a timestamp, and edit and delete icon buttons in the top-right corner (this matches the exact description in the review).
- Each card is edited in place. It has its own local edit-mode state and its own Speichern/Abbrechen buttons. This is independent of the Bearbeiten/Speichern batching of the Listing form from [[01-10-listing-views-overhaul]]. Notes save one by one. Each note is its own object, not a field on the Listing.
- Delete asks for confirmation. This matches the delete-confirmation pattern the app already uses elsewhere, for example for Criteria and Images.

---

## Verification

First, use `curl` directly on the new endpoints. Create, list, update, and delete a note on a real listing. Confirm that deleting the listing also deletes its notes (cascade). Then click through by hand: add a note, edit it, delete it. Confirm the list order (newest first). Confirm that notes of another Listing never show up.
