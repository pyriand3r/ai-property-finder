---
type: spec
created: 2026-07-28
title: List View & Ranking
project: property-finder-obs-style
spec: ranking-and-list-view
phase: 1
order: 5
category: listings
status: ready
updated: 2026-07-28
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - listings
  - ranking
---

## Overview

Two related but different views of a profile's Listings. See [[../CONTEXT#Ranking]].

---

## List View

A table you can sort and filter. It has the same sort options as the predecessor project: Score, price, size, date, and disaster risk (Phase 2). It has the same filter options: status, score range, price range, size range, and region. Every Listing appears here, whatever its Status.

**Filter behaviour** (client-side, in `ListingListTable.vue`; logic in `src/utils/listingFilters.ts`):

- Score, price, and size (`property_size_m2`) each take an optional min and max. Both bounds are inclusive.
- A Listing with no value for a field is hidden as soon as a bound on that field is set.
- Region is a dropdown. `listing.region` is free text, so the options are the distinct, trimmed values found in the profile's Listings.
- All filters combine with AND.

This view goes beyond the predecessor project's version. See [[01-10-listing-views-overhaul]] for full detail:

- A thumbnail column shows each Listing's title image (a placeholder if none is set).
- Selecting a row opens the Listing in a split-pane Detail View next to the list. It does not navigate to a separate page. The standalone `/listings/:id` route still exists for deep links (chat cards, bookmarks).
- Optional columns show the Grades of single Criteria. The user can turn them on or off. The choice is saved per browser.

---

## Ranking

The Ranking is the curated, narrated version of the `Rangliste` table in the reference Obsidian workflow:

| Column | Source |
|---|---|
| Rank | Row position after sorting by Score descending, without `rejected`/`archived` |
| Title, price, size, price/m² | Structured Listing fields |
| Note (score) | The computed 0–100 Score from `evaluate_listing` |
| Kurzfazit | A short one-liner written by the AI for each Listing, e.g. "Beste Infrastruktur, aber kleinstes Grundstück" (means: "best infrastructure, but smallest plot") |

**Regeneration**: When a Score changes (Weight edit, new Grade), the Rank order updates at once. This uses a plain SQL re-sort and makes no LLM call. The Kurzfazit text is not regenerated automatically. It is regenerated on request:

- **In the chat**: The user says „Rangliste aktualisieren“ (means: "update the ranking"). The Agent calls `compute_ranking` (deterministic, only Scores and sorting). Then it writes the Kurzfazit as part of its chat reply.
- **In the Ranking view**: There is a button „Rangliste aktualisieren“. The backend fetches the Scores and sends them to an isolated LLM call. It writes the Kurzfazit text back. No Agent is involved.

There are two code paths, and both are simple. The Agent writes the better Fazits, because it has context about the question. The button works without a running chat.

A profile-wide "Kurz-Fazit" summary block compares the top few Listings against each other, as in the reference Obsidian workflow. It is regenerated the same way: on request, via `compute_ranking` ([[../00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]]).
