---
type: spec
created: 2026-08-05
title: Listing List/Detail View Overhaul
project: property-finder-obs-style
spec: listing-views-overhaul
phase: 1
order: 10
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

A batch of UX fixes to the List View ([[01-05-ranking-and-list-view]]) and Detail View ([[01-07-manual-review-and-edit]]) found during a hands-on review pass after Layer 6. Grouped into one spec/build-slot because they're technically coupled, not just topically similar: the master-detail split requires extracting the Detail View's content into a reusable component, and every other Detail View change (reorder, status placement, edit mode, source link) lands on that same extracted component. Doing them as separate slots would mean touching the same files twice.

This spec **amends** [[01-05-ranking-and-list-view]] § List View and [[01-07-manual-review-and-edit]] § Editing Behaviour — those sections are updated in place alongside this one, not left contradicting it.

---

## Master-Detail Split Layout

The "Liste" tab (`ListingsPanel.vue`) changes from "click a row → navigate to `/listings/:id`" to a split layout: the list stays visible on the left, the selected Listing's full Detail View content renders on the right. Selection is local component state (`selectedListingId`), not a route change — clicking through several Listings in a row no longer means navigating away and back each time.

The standalone route `/listings/:listingId` is **not removed** — it's still how a deep link (e.g. the chat's inline Listing card, [[01-01-chat-interface]]) opens a Listing, and still works as a full-page view when reached directly. To avoid duplicating the Detail View's content between the route and the split pane, the existing `ListingDetailView.vue` page is split into:

- `ListingDetailPanel.vue` — the actual content (form, grade grid, document, notes), taking `listingId` as a prop. Reusable.
- `ListingDetailView.vue` — a thin route-level wrapper that reads `listingId` from the route param and renders `ListingDetailPanel`, plus the "← Zurück" back button (only meaningful in full-page context, not inside the split pane).

`ListingsPanel.vue`'s right pane renders `ListingDetailPanel` directly with the selected ID — no back button needed there, since "closing" a selection is just clicking a different row (or the same row again to deselect, showing an empty-state prompt).

---

## Thumbnails (List View + Detail View)

Both the List View table and the Detail View header show the Listing's title image, resolved the same way `ChatListingCard.vue` already does it (`listing.title_image_id` → matching `listing_image.file_path` → `/media/{file_path}`). List View shows a small thumbnail column; Detail View shows a larger one next to the title in the header.

No image at all (no `title_image_id` set) shows a neutral placeholder, not an empty cell — a Listing genuinely has no photo yet often enough (e.g. ingestion via PDF where no `download_image` call happened) that a blank-looking row/header would otherwise read as a rendering bug.

---

## Optional Per-Criterion Score Columns

The List View table gains a column-visibility control. Beyond the fixed columns (Titel, Status, Preis, Größe, Score, Angelegt), the user can toggle on one column per active Criterion in the profile, showing that Criterion's Grade for each Listing. Selection persists to `localStorage`, keyed by profile ID — this is a per-browser display preference, not profile data, and the app has no multi-user/account concept to hang a server-side preference off of.

---

## Detail View: Grade Table First, Status Prominent

`ListingGradeGrid` moves to the top of `ListingDetailPanel.vue`, above the form — the evaluation result is what most detail-view visits are actually checking, and the previous "form, then document, then grades at the bottom" order buried it.

`status` moves out of the field grid into the page header, next to the title — same visual tier as the existing `needs_reevaluation` ("Bewertung veraltet") tag, not one of eighteen fields in a two-column grid where it's easy to miss.

---

## Detail View: Explicit Edit/Save, Not Save-on-Blur

**This changes documented behavior in [[01-07-manual-review-and-edit]] § Editing Behaviour**, which currently says "Any field edit saves immediately (no separate 'save' step...)". That line is corrected in the same change as this spec.

`ListingForm.vue` gains a "Bearbeiten" button (top-right, alongside the header). Clicking it switches every field from read-only display to editable inputs and reveals "Speichern"/"Abbrechen". Every field changed while in edit mode is collected and sent as **one** `PATCH /api/listings/{id}` call on Speichern (not one PATCH per field, as today) — the backend endpoint already accepts a partial patch with multiple fields at once, so this is a frontend batching change only, no API shape change. Abbrechen discards the draft and reverts to the last-saved values without calling the API.

The `user_confirmed_fields` / `needs_reevaluation` side effects described in [[01-07-manual-review-and-edit]] § Editing Behaviour are unchanged in kind — they still fire per field that actually changed value, just computed once at Speichern time across the whole batch instead of once per blur event.

---

## Detail View: Clickable Source Link

`source_url`, in read-only/view mode, renders as `<a :href="source_url" target="_blank" rel="noopener">` instead of plain text. In edit mode (see above) it's still a plain text input, since it needs to be editable.

---

## Verification

`npm run dev`, manual click-through (no working browser automation in this project's sandbox — see prior layers' notes):

- Split-pane: select several Listings in a row, confirm the list stays visible and the right pane updates without a full navigation/reload
- Deep link: open `/listings/{id}` directly and via the chat's inline card, confirm the standalone page (with back button) still works
- Thumbnails render in both List View and Detail View; a Listing with no title image shows the placeholder, not a blank cell
- Toggle a per-criterion column on, reload the page, confirm the toggle persisted
- Grade grid appears above the form; status pill sits in the header next to the title
- Bearbeiten → change three fields across different sections → Speichern → confirm exactly one PATCH call fired (network tab) and all three changes landed; Bearbeiten → change a field → Abbrechen → confirm nothing was sent and the field reverted
- Click the source URL link, confirm it opens in a new tab
