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

A user-review finding: the Detail View has no way to jot down freeform running notes about a Listing ("called the agent, viewing booked for Saturday", "neighbor mentioned the roof was redone in 2019"). This is **not** the same thing as `seller_notes` — that's a single field about the seller/contact person, part of the seller-info group ([[01-07-manual-review-and-edit]]). Listing Notes are the user's own notes about the property, as many as they want, each its own timestamped, independently editable/deletable object.

The `listing_note` table already exists in the schema (carried over unchanged from dev-property-finder, see `backend/internal/db/migrations/000001_init.up.sql`) but has zero application code on top of it — this spec builds the full stack.

---

## Data Model

`listing_note`: `id, listing_id, content, created_at` today. This spec adds `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()` via a new migration (`000003_listing_note_updated_at`) — every other editable table in this schema already has one, and notes need it since they're edit-in-place, not delete-and-recreate.

No `title` field — a note is a single content block, same as the reference workflow's freeform per-listing notes.

---

## API

`internal/db/repository/note_repo.go` — mirrors the shape of `image_repo.go` (`Create`, `GetByID`, `ListByListing`, `Delete`), plus `Update` for in-place edits:

```go
type NoteRepository interface {
    Create(ctx context.Context, note *domain.ListingNote) error
    ListByListing(ctx context.Context, listingID uuid.UUID) ([]domain.ListingNote, error)
    Update(ctx context.Context, id uuid.UUID, content string) error
    Delete(ctx context.Context, id uuid.UUID) error
}
```

`internal/service/note_service.go` — thin orchestration (validate non-empty content, verify the listing exists on create), same shape as `internal/service/image_service.go`'s simpler paths.

`internal/api/notes.go` + routes, nested under the listing (matches the existing `/api/listings/{listingId}/images` pattern):

| Method | Path | |
|---|---|---|
| `GET` | `/api/listings/{listingId}/notes` | List, newest first |
| `POST` | `/api/listings/{listingId}/notes` | Create — body `{"content": "..."}` |
| `PATCH` | `/api/notes/{noteId}` | Update content |
| `DELETE` | `/api/notes/{noteId}` | Delete |

`listing_note.listing_id` already cascades on listing delete (`ON DELETE CASCADE`, confirmed in the existing migration) — no extra cleanup needed in `ListingService.Delete`.

---

## Frontend

A "Notizen" section in `ListingDetailPanel.vue` ([[01-10-listing-views-overhaul]]), separate from the seller-info field group:

- "Neue Notiz" button opens an inline empty card in edit mode
- Each note renders as its own card: content text, timestamp, edit and delete icon buttons in the top-right corner (matching the review's exact description)
- Edit-in-place per card (own local edit-mode state, own Speichern/Abbrechen) — independent of the Listing form's Bearbeiten/Speichern batching from [[01-10-listing-views-overhaul]]; notes save individually since each is its own object, not a field on the Listing
- Delete asks for confirmation (matches the app's existing delete-confirmation pattern elsewhere, e.g. Criteria/Images)

---

## Verification

`curl` against the new endpoints directly (create, list, update, delete a note on a real listing, confirm cascade-delete removes notes when the listing is deleted), then a manual click-through: add a note, edit it, delete it, confirm the list order (newest first) and that another Listing's notes never appear.
