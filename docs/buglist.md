---
type: tracking
title: Bug List
project: property-finder-obs-style
status: active
updated: 2026-08-06
tags:
  - property-finder-obs-style
  - bugs
---

## How to use this file

Found a bug while working on something else — not the thing you were explicitly asked to fix?
Write it here first, before investigating or fixing it, unless the user asks you to fix it right
away. One entry per bug: found-date and context, symptom, root cause if known, status, links to
any deeper write-up (spec, ADR).

Move an entry to **Fixed** once it's fixed and verified — link the fix's commit/spec there instead
of deleting the history of what it took to find and fix it.

---

## Open

### Criterion names show literal `&amp;` instead of `&`

- **Found**: 2026-08-06, live-verifying the Detail View grade grid ([[spec/01_mvp/01-10-listing-views-overhaul]])
- **Symptom**: criterion names containing an ampersand render as `Grundstück &amp; Lage` instead
  of `Grundstück & Lage`, in both the List View's optional grade columns and the Detail View's
  grade grid (`ListingGradeGrid.vue`).
- **Confirmed**: the literal `&amp;` is already present in the raw `GET
  /api/profiles/{id}/criteria` response — not a frontend rendering bug, the string is stored
  escaped in the `criterion.name` column.
- **Root cause**: not yet investigated. Suspect the name got HTML-escaped once at creation time
  (e.g. by a markdown-rendering step somewhere in the Agent's `propose_criteria` path) and the
  escaped string was persisted as-is, rather than escaping only ever happening at render time.
- **Status**: open, unfixed.

### Unicode escape sequences displayed literally in Confirm-Card

- **Found**: reported during Layer 6 review; full write-up in
  [[spec/01_mvp/01-12-chat-feedback-fixes]] § Unicode Escape Sequences Displayed Literally
- **Symptom**: a Confirm-Card once showed literal `\uXXXX` escape sequences instead of decoded
  umlauts in a proposed criterion's name/description.
- **Status**: could not reproduce on a fresh turn immediately after the report — diagnostic steps
  for whoever picks this up next are in the spec linked above. Stays open until reproduced.

---

## Fixed

(none yet)
