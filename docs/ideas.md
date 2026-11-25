---
type: tracking
title: Feature & Improvement Ideas
project: property-finder-obs-style
status: active
updated: 2026-11-24
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

### Full MCP coverage (everything the UI can do)

Asked on 2026-11-24, scoped down to evaluation only ([[adr/0026-external-evaluation-via-mcp]], [[spec/01_mvp/01-14-external-evaluation-via-mcp]]). The rest of the wish: profile, listing (incl. `ListingPatch`, status), note, image, Context replace/history, chat-session and read-only hazard/ranking tools on `/mcp`, plus MCP resources and prompts (`pf://profiles/{id}/criteria`, `evaluate_listing`). Each tool should call the same service as the REST handler (`backend/internal/service`, `backend/internal/mcpserver`). Decided limits that stay: no change of active Criteria over MCP ([[adr/0024-criterion-proposal-status]]), no delete tools. Ranking and rejection analysis as external operations (`get_ranking_brief` / `submit_ranking`) are also open. Builds on [[adr/0019-mcp-tool-interface]].

### Find the hazard region of a Listing

Found in #44. A Listing has a free-text `region`, a `country` and optional coordinates (`Listing` in `backend/internal/domain/types.go`, `src/types/listing.ts`). Hazard ratings are stored per region key such as `PT-1106` ([[adr/0025-hazard-ratings-region-level-static-data]], [[spec/01_mvp/01-13-hazard-ratings]]). Nothing maps one to the other. Issue #45 (hazard UI) ships the first option below as a stopgap, in `src/utils/hazard.ts` (`resolveHazardRegion`, rules in [[spec/01_mvp/01-13-hazard-ratings#UI]]): it matches `listing.region` by name, and shows no rating when the name is missing, not a municipality, or ambiguous. A stored key would replace it.

Options:

- Match `listing.region` against the names from `GET /api/hazards/regions?country=&q=`. It is cheap. It fails for regions that are not municipalities ("Alentejo") and is ambiguous for "Lagoa", which exists twice.
- Let the Agent resolve the municipality when it geocodes (`geocode` Tool, [[adr/0011-nominatim-geocoding-tool]]; Nominatim returns the municipality) and store the key on the Listing, for example in a new `listing.hazard_region_key` column.
- Bundle simplified CAOP municipality boundaries and look the key up from `latitude` and `longitude`. It needs no network and no Agent, but adds tens of MB.

Builds on: issue #45. Touches: the `listing` table and migration, `ListingService`, `ListingPatch` (a manual edit must be possible, [[adr/0001-chat-first-with-form-fallback]]), the Agent's `save_listing` Tool.

### Hazard lookup as an Agent Tool and an MCP tool

Found in #44. The Agent cannot answer "wie hoch ist das Erdbebenrisiko dort?" (means: how high is the earthquake risk there?) from the ratings. `HazardService.SearchRegions` and `GetRegionHazards` (`backend/internal/service/hazard_service.go`) already return everything a Tool needs. Add the Tool to `NewToolRegistry` (`backend/internal/service/tool_registry.go`, `backend/internal/agent/tools.go`). The MCP interface ([[adr/0019-mcp-tool-interface]], `backend/internal/mcpserver/`) reuses the same registry. The Tool is read-only, so it needs no Confirm-Card. It is most useful together with the entry above (the region of the current Listing). Related: issue #46 (done: `suggest_hazard_source`, see [[spec/01_mvp/01-15-hazard-source-catalogue]]).

### Hazard data for Madeira and the Azores (wildfire, flood, landslide)

Found in #44. Only earthquake covers the 30 island municipalities. The three other sources are mainland-only: the ICNF wildfire map, the APA flood hazard maps and ELSUS v2 ([[spec/01_mvp/01-13-hazard-ratings#Coverage]]). The flood research found that the Azores publish their own flood-risk areas and landslide data through a regional government service (`visualizador-idea.ambiente.azores.gov.pt`). It was not ingested. Madeira was not checked. Touches: `backend/internal/hazard/datasets/pt.json`, the coverage table in spec 01-13, and `TestLoad_EmbeddedPortugal_CoversEveryMunicipalityAsDocumented` in `backend/internal/hazard/hazard_test.go` (its rule says the islands have earthquake only).

### Use a Portuguese national landslide source instead of ELSUS

Found in #44. The landslide rating comes from the EU-level ELSUS v2 map because no national dataset could be downloaded: the ANEPC WFS refuses connections, the 2023 national risk assessment has only an image, and the REN layer has no unstable-slope breakdown ([[spec/01_mvp/01-13-hazard-ratings#Landslide (ELSUS v2, EU-level)]]). The issue named "EROS/CCDR" data, which no search found. If a national source becomes downloadable (a working ANEPC or APA service, or a REN layer split by land type), replace the source and its method text, and follow "Updating a dataset" in the same spec. Touches: `backend/internal/hazard/datasets/pt.json`, spec 01-13, [[adr/0025-hazard-ratings-region-level-static-data]] only if the decision changes.

### Keep the hazard dataset build scripts in the repository

Found in #44. `pt.json` was produced by one-off Python GIS scripts (raster and polygon overlays with the CAOP 2025 boundaries) that are not in the repository. Spec 01-13 lists every source, layer and threshold, so a person can repeat the work, but nobody can re-run it. A documented tool (inputs, pinned versions, one command per hazard) would make the next update, and the first one for Sweden or Austria (issue #46), much cheaper. `AGENTS.md` says there is no `scripts/` directory today, so first decide where such a tool lives and that it stays outside `go build ./...`. Touches: spec 01-13 "Updating a dataset", `backend/internal/hazard/datasets/`.

### Create a hazard source suggestion by hand, and link it to the dataset that ships it

Found in #46. A suggestion can only come from the Agent or an MCP client ([[adr/0027-hazard-source-catalogue-and-suggestions]], [[spec/01_mvp/01-15-hazard-source-catalogue]]). Two small gaps: (1) the user cannot add a source they know in the UI (a form on the "Naturgefahren-Quellen" section in `src/components/hazard/HazardSourcesPanel.vue` plus `POST /api/hazards/source-suggestions` calling `HazardSourceService.Suggest`); (2) when a maintainer ships an approved source, nothing marks the suggestion as done, so it stays `approved` forever. A fourth status `ingested`, or a `catalog_entry_id` column set by hand, would close it. Touches: migration for `hazard_source_suggestion`, `internal/service/hazard_source_service.go`, the panel above.

### Agent Tool to read the hazard source catalogue

Found in #46. `suggest_hazard_source` only learns that a source is known after it fails with "schon im Katalog". A read-only Tool (`list_hazard_sources`, country filter) built on `HazardSourceService.Catalog` would let the Agent check first and tell the user what coverage a country has. It could share the lookup Tool in "Hazard lookup as an Agent Tool and an MCP tool" above. Touches: `backend/internal/agent/tools.go`, `backend/internal/service/tool_registry.go`, `backend/internal/mcpserver/`.

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
