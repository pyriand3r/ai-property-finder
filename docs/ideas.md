---
type: tracking
title: Feature & Improvement Ideas
project: property-finder-obs-style
status: active
updated: 2026-08-06
tags:
  - property-finder-obs-style
  - ideas
---

## How to use this file

Idea for a feature or improvement found while working on something else? Write it here — don't
implement it without being asked, and don't let it block the task you were actually doing.
**Cross-reference every spec, ADR, layer, and file the idea would touch or depend on.** An idea
that isn't actionable later without re-deriving context (which files, which existing behavior it
changes, what it depends on) is a bad entry — write it the way you'd want to receive it cold.

Move an entry to **Done** once implemented, linking the commit/spec that did it.

---

## Open

### Give each ProfileShellView tab its own route so F5 doesn't reset to Chat

- **Where noticed**: 2026-08-06, user feedback — reloading the page (F5) always lands back on the
  Chat tab, even if Kontext/Kriterien/Inserate was open.
- **What**: `ProfileShellView.vue`'s `NTabs` (`type="line"`, `default-value="chat"`) has no
  `v-model:value` and isn't synced to the route at all — tab selection is purely local component
  state, lost on every reload. Only the profile itself is routable (`/profiles/:profileId`, see
  `src/router/index.ts`); Chat/Context/Kriterien/Inserate are not.
- **Depends on / touches**: `src/router/index.ts` (the single `profile-shell` route needs a tab
  segment, e.g. `/profiles/:profileId/:tab?` with `tab` restricted to
  `chat|context|criteria|listings`, defaulting to `chat` when absent — matches the four
  `NTabPane name="..."` values already used in `ProfileShellView.vue`). `ProfileShellView.vue`
  itself needs `NTabs` bound via `v-model:value` initialized from the route param and pushing a
  route change (probably `router.replace`, not `push`, so tab-switching doesn't spam browser
  history) on tab change. Note the "Context" tab (`NTabPane name="context"`) is currently just an
  `NEmpty` placeholder — "Context-Editor kommt in Layer 7" — so this idea is independent of
  Layer 7 landing, but Layer 7's Context Editor work should route through whatever `tab` param
  scheme this introduces rather than needing its own later.

### Translate List View's Status column to German

- **Where noticed**: 2026-08-06, live-verifying [[spec/01_mvp/01-10-listing-views-overhaul]] in the
  browser.
- **What**: `ListingListTable.vue`'s Status column renders the raw `ListingStatus` enum value
  (`new`, `waiting`, ...) instead of a German label. The Status **filter** dropdown right next to
  it already has the German labels (`statusOptions` in the same file — `Neu`, `Wartend`, `In
  Verhandlung`, ...); the table column just doesn't reuse that mapping.
- **Depends on / touches**: `src/components/listings/ListingListTable.vue` (`statusOptions` already
  has the label map, just needs a lookup used in the column's `render`). Same
  label-vs-value split exists in `ListingDetailPanel.vue`'s header status `NSelect` — could extract
  a shared `statusLabel` map into `src/utils/` at the same time, following the same pattern already
  used for `gradeColor` in `src/utils/gradeColor.ts` (centralized after it was needed in two
  components).

### Support clearing an already-set optional Listing field via PATCH

- **Where noticed**: 2026-08-06, reverting test data after live-verifying the Bearbeiten/Speichern
  batch-PATCH flow ([[spec/01_mvp/01-10-listing-views-overhaul]]).
- **What**: `PATCH /api/listings/{id}` with an explicit JSON `null` for an optional string field
  (e.g. `seller_phone`) does not clear it — Go's `encoding/json` unmarshals both `null` and an
  absent key into the same nil pointer on `*string` fields, and
  `backend/internal/service/listing_service.go`'s `applyPatch` only checks `if patch.Field != nil`,
  so a `null` is indistinguishable from "field not sent" and silently ignored. Confirmed live: had
  to fix a listing's `seller_phone` back to empty via a direct DB `UPDATE`, since the API offered no
  way to do it.
- **Depends on / touches**: `backend/internal/service/listing_service.go` (`ListingPatch` struct +
  `applyPatch`), `backend/internal/api/listings.go` (request body struct). Would need an explicit
  "clear this field" signal distinct from "omit this field" — e.g. a sentinel value, or switching
  optional fields to a wrapper type that distinguishes not-present from present-and-null (the
  standard Go PATCH-semantics problem). Frontend `ListingForm.vue` would also need a way to let the
  user actually clear a field (today's inputs can be emptied to `''`, not to `null`/undefined,
  which is a separate but related gap).

---

## Done

(none yet)
