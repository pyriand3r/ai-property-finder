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

Two related but distinct views over a profile's Listings — see [[../CONTEXT#Ranking]].

---

## List View

Sortable/filterable table, same sort options (Score, price, size, date, disaster risk in Phase 2), same filter options (status, score range, price range, size range, region) as [[../../dev-property-finder/spec/01_mvp/01-06-listing-list-view|dev-property-finder]]. Every Listing appears here regardless of Status.

Extended past dev-property-finder's version, see [[01-10-listing-views-overhaul]] for full detail:

- A thumbnail column shows each Listing's title image (placeholder if none set)
- Selecting a row opens the Listing in a split-pane Detail View alongside the list, not a separate page navigation — the standalone `/listings/:id` route still exists for deep links (chat cards, bookmarks)
- Optional, user-toggleable columns for individual Criteria's Grades, persisted per-browser

---

## Ranking

The curated, narrated equivalent of the reference workflow's `00_readme.md` Rangliste table ([[../../immobilienplan-schweden-portugal-oesterreich/schweden/00_readme#Kurzfassung & Rangliste]]):

| Column | Source |
|---|---|
| Rank | Row position after sorting by Score desc, excluding `rejected`/`archived` |
| Title, price, size, price/m² | Structured Listing fields |
| Note (score) | The computed 0–100 Score from `evaluate_listing` |
| Kurzfazit | Short AI-written one-liner per Listing, e.g. "Beste Infrastruktur, aber kleinstes Grundstück" |

**Regeneration**: Score changes (Weight edit, new Grade) update the Rank ordering immediately via plain SQL re-sort — no LLM call. The Kurzfazit text is regenerated on request, not automatically:

- **Im Chat**: User sagt „Rangliste aktualisieren“. Agent ruft `compute_ranking` (deterministisch, nur Scores + Sortierung), schreibt dann das Kurzfazit als Teil seiner Chat-Antwort.
- **In der Ranking-Ansicht**: Button „Rangliste aktualisieren“. Backend ruft Scores ab, sendet sie an einen isolierten LLM-Call, schreibt Kurzfazit-Text zurück. Kein Agent involviert.

Zwei Code-Pfade, beide einfach. Der Agent schreibt die besseren Fazits (hat Kontext zur Frage), der Button funktioniert ohne laufenden Chat.

A profile-wide "Kurz-Fazit" summary block (comparing the top few Listings against each other, as in the reference `00_readme.md`) is regenerated the same way — on request, via `compute_ranking` ([[../00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]]).
