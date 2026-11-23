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

_No open ideas._

---

## Done

### Give each ProfileShellView tab its own route so F5 doesn't reset to Chat

Done in #7. Route is `/profiles/:profileId/:tab?` with `tab` = `chat|context|criteria|listings|map`
(`map` is the later "Karte" tab). `ProfileShellView.vue` binds `NTabs` to `route.params.tab` and
switches with `router.replace`. A missing or unknown `tab` shows Chat.

### Translate List View's Status column to German

Done in #8. The Status column in `ListingListTable.vue` renders `statusLabel[row.status]` from
`src/utils/listingStatus.ts`, the same map the filter dropdown, the detail panel and the map
markers use. The filter dropdown is unchanged.
