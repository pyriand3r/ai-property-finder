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

### A chat session's history can end up with a dangling `tool_use` again, breaking every future turn in it

- **Found**: 2026-08-07, live-verifying Layer 8.2 (Listing-Context-Block) — sending a new message
  in an existing active session ("Schweden" profile, session `e278bb0a-45f7-41e3-a748-76656f1cba0d`).
- **Symptom**: the turn fails outright with a 400 from the Claude API:
  `messages.10: tool_use ids were found without tool_result blocks immediately after`. Every
  subsequent message sent to that same session fails the same way — the session is permanently
  stuck until "Neue Session" is used, since the invalid history is replayed on every turn.
- **Confirmed via `docker compose logs`**: `agent: turn failed for session ...: POST
  "https://api.anthropic.com/v1/messages": 400 Bad Request ... tool_use ids were found without
  tool_result blocks immediately after: 20af33ee-f72b-4ac8-95bc-50b8f763d401`.
- **Root cause**: not yet investigated. This is the same *symptom* as the bug
  [[spec/01_mvp/01-09-chat-reliability-fixes]] fixed (RF.1 — dangling `tool_use` blocks from the
  budget-exhausted branch of `loop.go`), but that fix specifically targeted the
  roundtrip-limit-hit path, and this session had already been through a lot of ad-hoc live-testing
  turns (fetch_url 403s, geocode, download_image validation errors, etc.) — the dangling call could
  be from a different, still-unfixed path where a tool_use gets persisted without ever being
  dispatched or without its result being persisted (e.g. a turn that errored/was interrupted
  mid-dispatch), or it could be stale data from before RF.1 shipped that's simply never been
  replayed again until now. Needs a fresh repro to tell which.
- **Status**: open, unfixed — worked around for the Layer 8.2 verification by starting a new
  session instead of debugging the corrupted one in place.

### Split-pane detail scroll falls through to page scroll near the bottom

- **Found**: 2026-08-06, reported by the user scrolling a long Listing in the master-detail split
  ([[spec/01_mvp/01-10-listing-views-overhaul]] § Master-Detail Split Layout).
- **Symptom**: scrolling inside the right-hand detail pane (`ListingDetailPanel.vue`) works
  correctly for roughly the first 3/4 of its content — the list and header stay in place, only the
  detail pane scrolls. Near the end of the detail content, scrolling stops staying contained and
  the whole page starts scrolling instead.
- **Root cause (hypothesis, not confirmed)**: `.listings-panel-split-detail` in
  `ListingsPanel.vue` is `position: sticky` with `max-height: calc(100vh - 2rem)` and
  `overflow-y: auto` — the intended pattern for an independently-scrolling sticky pane. But a
  `position: sticky` element can only stay stuck within the bounds of its **containing block**
  (here, the `.listings-panel-split` flex row). Since the pane's own max-height is set to nearly a
  full viewport, its containing block ends up almost exactly as tall as the pane itself, leaving
  very little scroll room between "the pane's own internal overflow-y:auto content finishes
  scrolling" and "the containing block's bottom edge is reached, so the sticky positioning gives
  up and the page takes over the scroll." That matches the reported "fine for 3/4, then the whole
  page scrolls" symptom.
- **Status**: open, unfixed. A fix likely needs to give `.listings-panel-split` a height tied to
  the viewport (not just its content) so the sticky pane has room to fully stick, or restructure
  the split away from `position: sticky` toward a fixed-height flex/grid shell with independent
  scroll regions (a more typical split-pane layout).

### Quell-URL link color hard to read in dark mode

- **Found**: 2026-08-06, reported by the user after the clickable source link shipped
  ([[spec/01_mvp/01-10-listing-views-overhaul]] § Detail View: Clickable Source Link).
- **Symptom**: the `<a>` tag rendering `listing.source_url` in view mode
  (`ListingForm.vue`) has no explicit color styling — it renders in the browser's default link
  blue, which is a dark, low-contrast blue against this app's dark theme.
- **Root cause**: `ListingForm.vue`'s Quell-URL `<a>` was added with `target="_blank" rel="noopener"`
  only, no class/style — never themed. The app has an existing dark/light theme setup
  (`usePreferredDark` composable, `NConfigProvider :theme`) that other components style against
  (e.g. `var(--n-color-target, ...)` fallbacks in `ListingListTable.vue`/`RankingView.vue`), this
  link just never got the same treatment.
- **Status**: open, unfixed.

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

### Stale "Bewertungstabelle" placeholder at the bottom of the Listing Document

- **Found**: 2026-08-06, reported by the user while reviewing the Detail View after
  [[spec/01_mvp/01-10-listing-views-overhaul]] shipped.
- **Symptom**: the Detail View still shows a "Bewertungstabelle" heading with the placeholder text
  "(wird nach Kriterien-Bewertung durch evaluate_listing ergänzt)" at the very bottom, inside the
  rendered `document_md` (`ListingDocument.vue`) — even though the real, structured grade grid
  (`ListingGradeGrid.vue`) now renders at the *top* of the panel per 01-10's "Grade Table First"
  change.
- **Root cause (likely)**: `document_md` is Agent-generated prose that itself embeds a
  "## Bewertungstabelle" section by design — see the system prompt instructions in
  `backend/internal/agent/context_builder.go` (lines ~35, ~70) and the seeded global context
  default in `backend/internal/db/migrations/000002_seed_global_context_defaults.up.sql` ("-
  Bewertungstabelle pro Kriterium"). That embedded section was always meant to be *the* grade
  display before 01-10 introduced the separate structured `ListingGradeGrid.vue` fed from
  `listing_criterion_grade` rows. 01-10 moved the structured grid to the top but never updated the
  Agent's document-generation instructions to drop (or stop populating) its own embedded copy —
  now there are two Bewertungstabelle sections, one live or a stale placeholder, and one likely
  redundant with the real grid once evaluated.
- **Status**: open, unfixed. Needs a decision: either stop instructing the Agent to embed its own
  Bewertungstabelle section in `document_md` (since `ListingGradeGrid.vue` is now the source of
  truth), or keep it but make clear it's redundant/for-the-original-text-context only.

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
