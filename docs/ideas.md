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

Did you find an idea for a feature or improvement while working on something else? Write it
here. Do not build it unless someone asks you to. Do not let it block the task you were doing.
**Link every spec, ADR, layer, and file that the idea would touch or depend on.** A bad entry
is one that cannot be acted on later without finding the context again (which files, which
existing behavior it changes, what it depends on). Write each entry the way you would want to
get it with no background.

When an idea is built, move its entry to **Done**. Link the commit or spec that did it.

---

## Open

### Give each ProfileShellView tab its own route so F5 doesn't reset to Chat

- **Where noticed**: 2026-08-06, user feedback. Reloading the page (F5) always goes back to the
  Chat tab, even if Kontext/Kriterien/Inserate was open.
- **What**: The `NTabs` in `ProfileShellView.vue` (`type="line"`, `default-value="chat"`) has no
  `v-model:value`. It is not linked to the route at all. The tab choice is only local component
  state, and it is lost on every reload. Only the profile itself has a route
  (`/profiles/:profileId`, see `src/router/index.ts`). Chat/Context/Kriterien/Inserate do not.
- **Depends on / touches**: `src/router/index.ts`. The single `profile-shell` route needs a tab
  segment, for example `/profiles/:profileId/:tab?`. `tab` is limited to
  `chat|context|criteria|listings` and defaults to `chat` when missing. This matches the four
  `NTabPane name="..."` values that `ProfileShellView.vue` already uses. `ProfileShellView.vue`
  itself needs `NTabs` bound with `v-model:value`, set from the route param. On a tab change it
  must push a route change. Use `router.replace`, not `push`, so that switching tabs does not
  fill the browser history. Note: the "Context" tab (`NTabPane name="context"`) is now only an
  `NEmpty` placeholder — "Context-Editor kommt in Layer 7" (means: "Context editor comes in
  Layer 7"). So this idea does not depend on Layer 7. But the Context Editor work in Layer 7
  should use the `tab` param scheme from this idea. It should not need its own scheme later.

### Translate List View's Status column to German

- **Where noticed**: 2026-08-06, while live-checking [[spec/01_mvp/01-10-listing-views-overhaul]]
  in the browser.
- **What**: The Status column in `ListingListTable.vue` shows the raw `ListingStatus` enum value
  (`new`, `waiting`, ...) and not a German label. The Status **filter** dropdown right next to
  it already has the German labels (`statusOptions` in the same file — `Neu`, `Wartend`, `In
  Verhandlung`, ...). The table column does not reuse that mapping.
- **Depends on / touches**: `src/components/listings/ListingListTable.vue` (`statusOptions`
  already has the label map. The column's `render` only needs a lookup that uses it). The same
  label-vs-value split exists in the header status `NSelect` of `ListingDetailPanel.vue`. You
  could also move a shared `statusLabel` map into `src/utils/` at the same time. Follow the
  pattern already used for `gradeColor` in `src/utils/gradeColor.ts` (it was centralized after
  two components needed it).

### Support clearing an already-set optional Listing field via PATCH

- **Where noticed**: 2026-08-06, while reverting test data after live-checking the
  Bearbeiten/Speichern batch-PATCH flow ([[spec/01_mvp/01-10-listing-views-overhaul]]).
- **What**: `PATCH /api/listings/{id}` with an explicit JSON `null` for an optional string field
  (for example `seller_phone`) does not clear it. Go's `encoding/json` turns both `null` and a
  missing key into the same nil pointer on `*string` fields. And `applyPatch` in
  `backend/internal/service/listing_service.go` only checks `if patch.Field != nil`. So a
  `null` looks the same as "field not sent" and is silently ignored. Confirmed live: a
  listing's `seller_phone` had to be set back to empty with a direct DB `UPDATE`, because the
  API had no way to do it.
- **Depends on / touches**: `backend/internal/service/listing_service.go` (`ListingPatch`
  struct + `applyPatch`), `backend/internal/api/listings.go` (request body struct). It needs an
  explicit "clear this field" signal that is different from "omit this field". For example, use
  a sentinel value. Or switch optional fields to a wrapper type that tells "not present" from
  "present and null" (the standard Go PATCH problem). The frontend `ListingForm.vue` also needs
  a way to let the user really clear a field. Today the inputs can be emptied to `''`, not to
  `null`/undefined. That is a separate but related gap.

---

## Done

(none yet)
