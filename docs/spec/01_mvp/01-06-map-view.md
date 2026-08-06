---
type: spec
created: 2026-07-28
title: Map View
project: property-finder-obs-style
spec: map-view
phase: 1
order: 6
category: listings
status: ready
updated: 2026-08-06
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - listings
  - map
---

## Overview

OSM + OpenLayers, one marker per Listing with coordinates, colour-coded by Status. Master-detail
split layout, same pattern as the List View ([[01-10-listing-views-overhaul]]) — map on the left,
selected Listing's full `ListingDetailPanel.vue` on the right, no page navigation on selection.
Originally modelled on the reference workflow's
[[../../immobilienplan-schweden-portugal-oesterreich/schweden/00_karte.html]]; the split-pane
layout supersedes that reference's popup-only interaction once List View established the
master-detail pattern first.

---

## Layout

`ListingsMapPanel.vue` — a new "Karte" tab in `ProfileShellView.vue`, alongside Chat/Context/
Kriterien/Inserate. Fetches its own Listing set via `useListings` (independent of the Inserate
tab's own fetch — each tab owns its data, no cross-tab coupling). Renders `ListingMap.vue` inside
`ListingSplitPane.vue`, the same shared split-layout component the List View's "Liste" tab uses
(extracted from `ListingsPanel.vue` once Map View needed the identical sticky-right-pane/
empty-state/`:key`-forced-remount pattern — see that component for why the remount key is
necessary).

`ListingMap.vue` is heavy (OpenLayers, ~300kB) and lazy-loaded via `defineAsyncComponent` in
`ProfileShellView.vue` — the other tabs shouldn't pay for it until the Karte tab is actually
opened.

**Known issue**: the split pane's right-hand scroll falls through to page scroll near the bottom
of long content — same bug as the List View's split pane, tracked in
[[../../buglist#Split-pane detail scroll falls through to page scroll near the bottom]]. Not
specific to Map View, not fixed here.

---

## Markers

- One marker per Listing with both `latitude` and `longitude` set. Coordinates are populated by
  the Agent via the `geocode` Tool (Nominatim/OSM, [[../../docs/adr/0011-nominatim-geocoding-tool]])
  during or after ingestion.
- Listings without coordinates (Agent couldn't geocode the address, or no address was present)
  simply don't appear on the map; the List View remains the complete record.
- Marker color follows `listing.status`, using the shared `statusColor` map in
  `src/utils/listingStatus.ts` (also home to the shared `statusLabel`/`statusOptions`,
  centralized there once needed in more than one place — see `ListingListTable.vue`/
  `ListingDetailPanel.vue`'s status dropdowns).
- The selected Listing's marker renders larger with a white outline; selection state is pushed
  into the map via a prop (`selectedListingId`), not re-derived from map state — the split pane's
  right side is the source of truth for "what's selected."
- On load, the map view fits its extent to all geocoded markers (`ol/extent.boundingExtent` +
  `View.fit`); with no geocoded Listings it falls back to a default center over Sweden (most
  seeded data is Swedish) at a low zoom.

---

## Marker Click Behavior

Clicking a marker does two things simultaneously:

1. **Selects the Listing** — emits `select(listingId)`, exactly like `ListingListTable.vue`'s row
   click, driving the same `ListingSplitPane`'s right-hand `ListingDetailPanel`.
2. **Opens a small popup** anchored to the marker (an OpenLayers `Overlay`) showing: title, price,
   score, and a 100x100 thumbnail (same `useListingThumbnails` composable as List View/Rangliste/
   Detail View, resolving `title_image_id` → `/media/{file_path}`). The popup has its own close
   button; clicking empty map area also closes it.

This differs from the List View, which has no per-row popup — the map's popup exists because a
marker click doesn't otherwise give at-a-glance context the way a table row's visible columns do.

---

## Open Questions / TBD

- Whether to add a "Karte aktualisieren" chat affordance for bulk re-geocoding if several older
  Listings are missing coordinates, or handle it per-Listing via a Follow-Up Instruction
