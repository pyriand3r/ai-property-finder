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

The map uses OSM and OpenLayers. Each Listing with coordinates gets one marker. The marker colour shows the Status. The layout is a master-detail split, the same pattern as the List View ([[01-10-listing-views-overhaul]]). The map is on the left. The full `ListingDetailPanel.vue` of the selected Listing is on the right. Selecting a Listing does not navigate to a new page.
The view was first modelled on the map of the reference Obsidian workflow. The split-pane
layout replaces that reference's popup-only interaction. The List View set the master-detail pattern first.

---

## Layout

`ListingsMapPanel.vue` is a new "Karte" tab in `ProfileShellView.vue`. It sits next to Chat/Context/
Kriterien/Inserate. It fetches its own Listing set with `useListings`. This is separate from the Inserate tab's fetch. Each tab owns its data, so tabs are not coupled. It renders `ListingMap.vue` inside
`ListingSplitPane.vue`. This is the same shared split-layout component that the "Liste" tab of the List View uses. It was extracted from `ListingsPanel.vue` when Map View needed the same pattern: a fixed-height shell with independently scrolling left and right panes, an empty state, and a `:key`-forced remount. See that component for why the remount key is
needed.

`ListingMap.vue` is heavy (OpenLayers, ~300kB). It is lazy-loaded with `defineAsyncComponent` in
`ProfileShellView.vue`. The other tabs should not pay for it until the Karte tab is opened.

**Known issue**: The right-hand scroll of the split pane falls through to the page scroll near the bottom
of long content. This is the same bug as in the List View's split pane. It is tracked in
[[../../buglist#Split-pane detail scroll falls through to page scroll near the bottom]]. It is not specific to Map View, and it is not fixed here.

---

## Markers

- There is one marker per Listing that has both `latitude` and `longitude` set. The Agent fills in the coordinates with the `geocode` Tool (Nominatim/OSM, [[../../docs/adr/0011-nominatim-geocoding-tool]]). It does this during or after ingestion.
- Listings without coordinates do not appear on the map. This happens when the Agent could not geocode the address, or when there was no address. The List View stays the complete record.
- The marker colour follows `listing.status`. It uses the shared `statusColor` map in
  `src/utils/listingStatus.ts`. That file also holds the shared `statusLabel` and `statusOptions`. They were
  moved there once more than one place needed them. See the status dropdowns in `ListingListTable.vue` and
  `ListingDetailPanel.vue`.
- The selected Listing's marker is drawn larger, with a white outline. The selection state is pushed
  into the map through a prop (`selectedListingId`). It is not worked out again from the map state. The right side of the split pane is the source of truth for "what is selected."
- On load, the map fits its extent to all geocoded markers (`ol/extent.boundingExtent` +
  `View.fit`). If no Listing is geocoded, it falls back to a default centre over Sweden (most
  seeded data is Swedish) at a low zoom.

---

## Marker Click Behavior

A click on a marker does two things at the same time:

1. **Selects the Listing.** It emits `select(listingId)`, just like the row
   click in `ListingListTable.vue`. This drives the right-hand `ListingDetailPanel` of the same `ListingSplitPane`.
2. **Opens a small popup** anchored to the marker (an OpenLayers `Overlay`). The popup shows the title, price,
   score, and a 100x100 thumbnail. The thumbnail uses the same `useListingThumbnails` composable as the List View, Rangliste, and
   Detail View. It resolves `title_image_id` → `/media/{file_path}`. The popup has its own close
   button. A click on an empty map area also closes it.

This differs from the List View, which has no popup per row. The map needs a popup because a marker click gives no quick overview. A table row gives this through its visible columns.

---

## Open Questions / TBD

- Should we add a "Karte aktualisieren" chat action? It would re-geocode in bulk when several older
  Listings have no coordinates. The other option is to handle each Listing with a Follow-Up Instruction.
