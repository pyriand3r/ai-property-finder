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

This spec is a batch of UX fixes to the List View ([[01-05-ranking-and-list-view]]) and the Detail View ([[01-07-manual-review-and-edit]]). We found them in a hands-on review after Layer 6. They share one spec and one build slot because they are technically linked, not just similar in topic. The master-detail split needs the Detail View's content in a reusable component. Every other Detail View change (reorder, status placement, edit mode, source link) lands on that same component. Separate slots would mean changing the same files twice.

This spec **amends** [[01-05-ranking-and-list-view]] § List View and [[01-07-manual-review-and-edit]] § Editing Behaviour. Those sections are updated in place together with this one, so they do not contradict it.

---

## Master-Detail Split Layout

Today the "Liste" tab (`ListingsPanel.vue`) works like this: click a row, then navigate to `/listings/:id`. It changes to a split layout. The list stays visible on the left. The full Detail View content of the selected Listing shows on the right. Selection is local component state (`selectedListingId`), not a route change. Clicking through several Listings in a row no longer means leaving the page and coming back each time.

The standalone route `/listings/:listingId` is **not removed**. A deep link still uses it to open a Listing (for example the inline Listing card in the chat, [[01-01-chat-interface]]). It still works as a full-page view when opened directly. The route and the split pane must not duplicate the Detail View's content. So the existing `ListingDetailView.vue` page is split into two parts:

- `ListingDetailPanel.vue` holds the actual content (form, grade grid, document, notes). It takes `listingId` as a prop. It is reusable.
- `ListingDetailView.vue` is a thin wrapper for the route. It reads `listingId` from the route param and renders `ListingDetailPanel`. It also renders the "← Zurück" back button (meaning "back"). The button only makes sense in full-page context, not inside the split pane.

The right pane of `ListingsPanel.vue` renders `ListingDetailPanel` directly with the selected ID. It needs no back button. To "close" a selection, the user clicks a different row. Clicking the same row again deselects it and shows an empty-state prompt.

---

## Thumbnails (List View + Detail View)

The List View table and the Detail View header both show the title image of the Listing. They find it the same way `ChatListingCard.vue` already does: `listing.title_image_id` → matching `listing_image.file_path` → `/media/{file_path}`. The List View shows a small thumbnail column. The Detail View shows a larger thumbnail next to the title in the header.

If there is no image (no `title_image_id` set), show a neutral placeholder, not an empty cell. A Listing often has no photo yet (for example, ingestion from a PDF where no `download_image` call happened). A blank row or header would then look like a rendering bug.

---

## Optional Per-Criterion Score Columns

The List View table gets a control for column visibility. The fixed columns are Titel, Status, Preis, Größe, Score, Angelegt. Beyond these, the user can switch on one column per active Criterion in the profile. That column shows the Grade of that Criterion for each Listing. The selection is saved to `localStorage`, keyed by profile ID. This is a display preference per browser, not profile data. The app has no multi-user or account concept, so there is nowhere to store a server-side preference.

---

## Detail View: Grade Table First, Status Prominent

`ListingGradeGrid` moves to the top of `ListingDetailPanel.vue`, above the form. Most visits to the Detail View check the evaluation result. The old order was form, then document, then grades at the bottom. That order hid the result.

The rendered Listing Document (`ListingDocument.vue`) hides its own "Bewertungstabelle" section, because the grade grid is now the source of truth (issue #5). The stored Markdown and the edit view keep it.

`status` moves out of the field grid into the page header, next to the title. It gets the same visual level as the existing `needs_reevaluation` tag ("Bewertung veraltet", meaning "evaluation outdated"). It is no longer one of eighteen fields in a two-column grid, where it is easy to miss.

---

## Detail View: Explicit Edit/Save, Not Save-on-Blur

**This changes documented behavior in [[01-07-manual-review-and-edit]] § Editing Behaviour.** That section now says "Any field edit saves immediately (no separate 'save' step...)". We correct that line in the same change as this spec.

`ListingForm.vue` gets a "Bearbeiten" button (meaning "edit"), top-right, next to the header. Clicking it switches every field from read-only display to editable inputs. It also shows the buttons "Speichern" (meaning "save") and "Abbrechen" (meaning "cancel"). While in edit mode, the form collects every changed field. On Speichern it sends them all in **one** `PATCH /api/listings/{id}` call. Today it sends one PATCH per field. The backend endpoint already accepts a partial patch with several fields at once. So this is a frontend batching change only. The API shape does not change. Abbrechen throws away the draft and goes back to the last saved values. It does not call the API.

The side effects on `user_confirmed_fields` and `needs_reevaluation` are described in [[01-07-manual-review-and-edit]] § Editing Behaviour. They stay the same in kind. They still fire for each field whose value really changed. The only difference is timing. They are computed once at Speichern time for the whole batch, not once per blur event.

---

## Detail View: Clickable Source Link

In read-only/view mode, `source_url` renders as `<a :href="source_url" target="_blank" rel="noopener">` instead of plain text. In edit mode (see above) it stays a plain text input, because the user must be able to edit it.

---

## Verification

Run `npm run dev` and click through by hand. This project's sandbox has no working browser automation (see the notes of earlier layers):

- Split-pane: select several Listings in a row. Confirm the list stays visible. Confirm the right pane updates without a full navigation or reload.
- Deep link: open `/listings/{id}` directly and through the inline card in the chat. Confirm the standalone page (with back button) still works.
- Thumbnails show in both the List View and the Detail View. A Listing with no title image shows the placeholder, not a blank cell.
- Switch a per-criterion column on, reload the page, and confirm the setting stayed.
- The grade grid shows above the form. The status pill sits in the header next to the title.
- Bearbeiten → change three fields in different sections → Speichern. Confirm exactly one PATCH call fired (network tab) and all three changes were saved. Bearbeiten → change a field → Abbrechen. Confirm nothing was sent and the field went back to its old value.
- Click the source URL link. Confirm it opens in a new tab.
