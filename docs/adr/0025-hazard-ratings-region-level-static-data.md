---
type: adr
title: "Natural-hazard ratings are region-level, 1-5, static data cached in the database"
project: property-finder-obs-style
status: accepted
created: 2026-11-24
tags:
  - property-finder-obs-style
  - adr
  - hazard
---

## Status

Accepted. Affects [[../../spec/00_infrastructure_setup/00-02-database-schema]] and [[../../spec/01_mvp/01-13-hazard-ratings]]. Decided in issue #44, the first sub-issue of #17. The UI is issue #45.

## Context

A buyer wants to know how exposed a property is to wildfire, flood, earthquake and landslide. This must be visible **apart from the Score**. The Score says how well a listing fits the user's wishes. A natural hazard is a fact about a place.

Official hazard data is published as national maps or tables. It is not published per listing. Each source also uses its own units: a five-class map, seismic zones with a ground acceleration, flood polygons around "significant risk" areas. Asking a live service for every listing would be slow, could fail, and would give answers that change without notice. The first country is Portugal. Sweden and Austria follow later (issue #46).

## Decision

1. **A rating belongs to a region and a hazard type, not to a listing.** For Portugal the region is the municipality (*concelho*). Its key is `PT-<DICO>`, for example `PT-1106` for Lisboa. The DICO code is the official 4-digit municipality code of the national map (CAOP). Names are not unique ("Lagoa" exists twice), so the key is the identity.
2. **The scale is 1 to 5, and higher is worse.** 1 = very low hazard, 5 = very high. This is the opposite direction of the Score. A hazard rating is never an input to the Score, the ranking or `evaluate_listing`, and no table has a foreign key to `listing`.
3. **Three tables** (migration `000005_hazard_rating`): `hazard_source` (where the data comes from), `hazard_region` (the places) and `hazard_rating` with `region_key`, `hazard_type`, `rating`, `source_id` and `updated_at`. The database enforces the rules: rating 1-5, the four hazard types, a region key that starts with the country code, and a rating that can only cite a source of its own hazard type.
4. **The data is a static cache, not a live lookup.** One JSON file per country is embedded in the binary (`internal/hazard/datasets`). At boot, `HazardService.SeedStatic` makes the tables mirror those files in one transaction: it upserts what the files hold and deletes what they no longer hold. There is no network access at runtime and no request ever writes these tables. A boot with unchanged files writes nothing, so `updated_at` records the last real change.
5. **Every source says how it became a rating.** `hazard_source` keeps publisher, URL, licence, dataset version, retrieval date and `method`. `method` states which step is the app's own interpretation and not an official scale. Municipal ratings are mostly such an interpretation: the sources give pixels, polygons or zones, and the app reduces them to one number per municipality.
6. **No data means no row.** If a source does not cover a region (for example the Portuguese islands for wildfire), that rating does not exist. The app never fills in a default.
7. **Read API.** `GET /api/hazards/regions?country=&q=` finds regions by name. `GET /api/hazards/regions/{regionKey}` returns one region with its ratings and their sources. Finding the region of a *listing* is not part of this decision.

## Consequences

- Updating a dataset means replacing its JSON file and releasing. The next boot syncs it. A file that fails validation (`hazard.Parse`) stops the boot. A test validates the files that ship.
- Ratings are **coarse**. One number per municipality hides soil, local geology, the distance to a river and the difference between a city centre and the countryside. The docs say so for every source. The UI (#45) must not present a rating as a guarantee.
- A low rating can mean "no data signal" and not "safe". Example: flood rating 1 means the national flood agency designated no significant flood-risk area. It does not mean flooding is impossible. See the Portugal section of the spec.
- The islands (Madeira, Azores) only have the hazards their sources cover. Missing ratings are absent from the API response.
- The generation of a dataset (GIS overlay of national maps with municipality boundaries) is **not** part of the repository. The spec records the sources, layers, thresholds and dates, so a later maintainer can repeat it.
- A fifth hazard type needs one new migration (replace the `hazard_type` CHECK on `hazard_source`) and one constant in `domain`.
- Another country needs a new dataset file and a new region key scheme `<country>-<code>`. No schema change.
- Matching a listing to a region (by name, or by coordinates with municipality boundaries) is open. Issue #45 needs it. It is recorded in `docs/ideas.md`.

## Alternatives considered

- **Query a live hazard service per listing**: rejected. It adds latency, an outage mode and licence questions to every listing view. The issue asks for cached static data.
- **Store the ratings on the listing**: rejected. A rating describes a place that many listings share. Updating a dataset would have to touch every listing, and the number would invite mixing into the Score.
- **Fold the ratings into the Score as one more Criterion**: rejected. The requirement is to show them separately, and the scales point in opposite directions.
- **Seed through a SQL migration**: rejected. Migrations are forward-only and run once, so a corrected dataset could not be shipped. A migration with 1,200 inserts is also unreadable.
- **Keep the files on the `/data` volume**: rejected. The data must be versioned and released with the code that reads it, and it must exist on a fresh install with no volume set up.
- **One flat table with the source and the region name repeated on each row**: rejected. A source is documented once, and a rating row should stay small.
- **PostgreSQL ENUM for the hazard type**: rejected, for the same reason as in ADR 0024. TEXT with a CHECK constraint is easier to extend.
- **Point-in-polygon lookup from coordinates**: not now. It needs official boundaries bundled with the app (tens of MB) and a spatial step per listing.
