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

Every field the Agent writes must be traceable and correctable outside the chat. This is the app's transparency guarantee: a conversational ingestion pipeline is only trustworthy if the user can always fall back to a plain form and see/fix exactly what's stored.

---

## Listing Detail View

Plain form view of every structured field (title, price, sizes, address, coordinates, seller info, status, notes) plus a rendered view of `document_md` and the Criteria grade grid — same layout intent as [[../../dev-property-finder/spec/01_mvp/01-12-rate-listing-criteria#Detail View|dev-property-finder's detail view]], with `document_md` added above the grade grid.

Reachable both from List/Ranking/Map and via the inline card in a chat turn (see [[01-01-chat-interface]]).

---

## Editing Behaviour

- Any field edit saves immediately (no separate "save" step for the form as a whole — matches the rest of the app's low-friction feel)
- Editing a field appends its name to `listing.user_confirmed_fields` ([[../00_infrastructure_setup/00-02-database-schema#listing]])
- A subsequent Agent Follow-Up Instruction that touches a `user_confirmed_fields` entry asks for confirmation before overwriting it ("Preis wurde von dir manuell auf 310.000 kr gesetzt — soll ich das überschreiben?") instead of silently clobbering a manual correction. Fields never manually touched are updated freely
- Editing any structured field that participates in Criteria evaluation sets `listing.needs_reevaluation = true`. The user triggers re-evaluation manually from the detail view button or the list view banner ([[../../docs/adr/0015-manual-bulk-reevaluation]]).
- Editing `document_md` directly is also supported (plain Markdown textarea) — it does not itself trigger re-evaluation, since it's prose, not a structured field

---

## Status Management

Unchanged from [[../../dev-property-finder/spec/01_mvp/01-08-listing-status-management|dev-property-finder]]: `new`, `waiting`, `in_negotiation`, `visited`, `rejected`, `archived`, `rejected` requires a comment. Settable both here and conversationally ("das ist raus, zu teuer für die Fläche" → Agent asks to confirm setting Status to `rejected` with that as the comment).

---

## Open Questions / TBD

- Whether `user_confirmed_fields` should ever auto-clear (e.g. after N months) or stay permanent until the user removes it themselves — leaning permanent, since a manual correction rarely becomes wrong again on its own
