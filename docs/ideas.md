---
type: tracking
title: Feature & Improvement Ideas
project: property-finder-obs-style
status: active
updated: 2026-11-23
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

---

## Done

(none yet)
