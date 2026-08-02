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
updated: 2026-07-28
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - listings
  - map
---

## Overview

Unchanged in substance from [[../../dev-property-finder/spec/01_mvp/01-07-listing-map-view|dev-property-finder]]: OSM + OpenLayers, one marker per Listing with coordinates, colour-coded by Status, popup with key facts and a link to the detail view — directly modelled on the reference workflow's [[../../immobilienplan-schweden-portugal-oesterreich/schweden/00_karte.html]].

---

## What's New

- Coordinates are populated by the Agent via the `geocode` Tool (Nominatim / OSM, [[../../docs/adr/0011-nominatim-geocoding-tool]]) during or after ingestion — the Agent resolves the listing address to lat/lon as part of the ingestion flow
- Listings without coordinates (Agent couldn't geocode the address, or no address was present) simply don't appear on the map; the List view remains the complete record
- Marker popup mirrors the reference workflow's popup fields: price, rank, area, plot, rooms, price/m², heating, water/sewage, Status badge, link to detail view

---

## Open Questions / TBD

- Whether to add a "Karte aktualisieren" chat affordance for bulk re-geocoding if several older Listings are missing coordinates, or handle it per-Listing via a Follow-Up Instruction
