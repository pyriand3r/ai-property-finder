---
type: spec
created: 2026-11-24
title: Natural Hazard Ratings
project: property-finder-obs-style
spec: hazard-ratings
phase: 1
order: 13
category: listings
status: ready
updated: 2026-11-24
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - listings
  - hazard
---

## Overview

A buyer wants to know how exposed a place is to **wildfire, flood, earthquake and landslide**. The app answers this with a **Hazard Rating** per region and hazard type. See [[../CONTEXT#Hazard Rating]].

Four rules shape everything below. They are decided in [[../../docs/adr/0025-hazard-ratings-region-level-static-data]].

- The rating belongs to a **region**, not to a Listing. Many listings share one region.
- It is **separate from the Score**. It is never an input to the Score, the ranking or `evaluate_listing`. No table has a foreign key to `listing`.
- The data is **static**. It ships inside the backend binary and is cached in the database at boot. Nothing is fetched at runtime, and no request writes it.
- Each rating names its **source** and how the source's own values became a number from 1 to 5.

This spec covers the backend: data model, dataset files, sync, read API and the Portugal data (issue #44). The UI is issue #45. More countries and Agent-suggested sources are issue #46.

## Concepts

**Hazard type.** One of `wildfire`, `flood`, `earthquake`, `landslide`. The API lists them in this order.

**Rating.** A whole number from 1 to 5. **Higher is worse**, the opposite direction of the Score.

| Rating | Meaning |
|---|---|
| 1 | very low hazard |
| 2 | low |
| 3 | medium |
| 4 | high |
| 5 | very high |

A rating says where a region sits on **its own source's scale**. A 3 for earthquake and a 3 for flood do not describe the same size of risk, so never add or average ratings of different hazard types.

**Region.** The geographic unit a rating is given for. Its key is `<country>-<code>`: the ISO 3166-1 alpha-2 country code, a dash, and the national code of the unit, in upper case. For Portugal the unit is the municipality (*concelho*) and the code is its 4-digit DICO code, for example `PT-1106` for Lisboa. Names are not unique ("Lagoa" exists in Faro and on São Miguel), so the key is the identity. `parent_name` (district or island) tells such regions apart.

**Source.** The published dataset a rating was derived from. A source has a publisher, a URL, a licence, a dataset version, a retrieval date and a `method`. The `method` says how the dataset's own values became the 1-5 scale. It says which step is the app's own interpretation and not an official scale.

**No data means no rating.** If no source covers a region and hazard type, the rating does not exist. The app does not guess.

## Data model

Three tables, created by migration `000005_hazard_rating`. The columns are in [[../00_infrastructure_setup/00-02-database-schema#Hazard tables]].

## Dataset files

One JSON file per country sits in `backend/internal/hazard/datasets/` and is embedded in the binary (`go:embed`). The file for Portugal is `pt.json`.

```json
{
  "country": "PT",
  "sources": [
    {
      "id": "pt-icnf-wildfire-hazard-2020-2030",
      "hazard_type": "wildfire",
      "title": "...", "publisher": "...", "url": "https://...", "license": "...",
      "dataset_version": "...", "retrieved_on": "2026-11-24",
      "method": "...", "notes": "..."
    }
  ],
  "regions": [
    {"key": "PT-1106", "name": "Lisboa", "parent_name": "Lisboa",
     "ratings": {"wildfire": 2, "flood": 2, "earthquake": 3, "landslide": 3}}
  ]
}
```

A hazard type missing from a region's `ratings` means "no data". A dataset has **exactly one source per hazard type**, so a rating does not repeat its source id; the loader fills it in.

`hazard.Parse` rejects a file, and so stops the backend at boot, when:

- the JSON has unknown fields or data after the object;
- the country is not a two-letter code, or the file has no regions or no sources;
- a source has an empty field, an unknown hazard type, a URL that is not http(s), or a `retrieved_on` that is not `YYYY-MM-DD`;
- two sources share an id, or two sources have the same hazard type;
- a source is not used by any rating;
- a region key is malformed, does not start with the country code, or appears twice;
- a region has no name or no ratings;
- a rating is outside 1-5, names an unknown hazard type, or has no source for its hazard type.

## Sync at boot

`main` calls `HazardService.SeedStatic` right after the migrations. For each dataset it runs one transaction that makes the tables mirror the file:

1. Upsert the sources, the regions and the ratings.
2. Delete the country's ratings that are no longer in the file, then its regions that are no longer in the file, then the sources no rating cites.

Data of other countries is never touched. A boot with unchanged files writes no rating row, so `hazard_rating.updated_at` is the time the rating or its source id last changed. A failed sync stops the boot, like a failed migration.

To change data, replace the file and release the backend. Editing the tables by hand is overwritten at the next boot.

## API

Both endpoints are read-only and sit behind the login like the rest of `/api` (ADR 0018). Neither reads a Listing.

### `GET /api/hazards/regions`

Finds regions, for example to get the key of a place known by name.

| Query | Meaning |
|---|---|
| `country` | Optional. Two letters, any case (`pt`). Anything else is a 400. |
| `q` | Optional. Keeps regions whose name contains it. Case, accents and hyphens are ignored, so `evora` finds Évora. |

The answer is a list ordered by name, `[]` when nothing matches:

```json
[
  {"key": "PT-0806", "country": "PT", "name": "Lagoa", "parent_name": "Faro"},
  {"key": "PT-4201", "country": "PT", "name": "Lagoa", "parent_name": "Ilha de São Miguel"}
]
```

The list is not paged. A country has a few hundred regions at most.

### `GET /api/hazards/regions/{regionKey}`

One region with its ratings, in hazard type order, each with its source. The key is case-insensitive. A malformed key is a 400, an unknown key is a 404.

```json
{
  "key": "PT-1106", "country": "PT", "name": "Lisboa", "parent_name": "Lisboa",
  "ratings": [
    {
      "hazard_type": "wildfire", "rating": 2, "updated_at": "2026-11-24T06:46:18+01:00",
      "source": {
        "id": "pt-icnf-wildfire-hazard-2020-2030", "hazard_type": "wildfire",
        "title": "...", "publisher": "...", "url": "https://...", "license": "...",
        "dataset_version": "...", "retrieved_on": "2026-11-24", "method": "...", "notes": "..."
      }
    }
  ]
}
```

A hazard the datasets have no data for is missing from `ratings`. `ratings` is never `null`.

## Portugal datasets

Region list, names and boundaries come from the **CAOP 2025** (Carta Administrativa Oficial de Portugal) of the DGT: 308 municipalities. The 278 mainland ones come from `https://ogcapi.dgterritorio.gov.pt/collections/municipios` and Madeira (11) and the Azores (19) from the CAOP 2025 GeoPackages `CAOP_RAM_2025-gpkg.zip` and `CAOP_RAA_2025-gpkg.zip` at `https://geo2.dgterritorio.gov.pt/caop/`. The region key is the CAOP field `dtmn`. CAOP is CC BY 4.0. All data below was retrieved on 2026-11-24.

The rating counts are municipalities per rating 1 / 2 / 3 / 4 / 5.

### Wildfire (ICNF)

| | |
|---|---|
| Source id | `pt-icnf-wildfire-hazard-2020-2030` |
| Dataset | *Carta de Perigosidade de Incêndio Rural 2020-2030*, the structural rural-fire hazard map of ICNF (Instituto da Conservação da Natureza e das Florestas), published in Diário da República in Aviso n.º 6345/2022 |
| Files | `https://geocatalogo.icnf.pt/dados/perigosidade_estrutural_2020_2030.zip` (25 m GeoTIFF), landing page `https://geocatalogo.icnf.pt/metadados/perigosidade_estrutural_20_30.html` |
| Licence | Free open data with attribution, indicative only |
| Native values | 5 classes: 1 Muito Baixa, 2 Baixa, 3 Média, 4 Alta, 5 Muito Alta. Value 15 is no data (urban land, water). |
| Rule | Lay the raster over the CAOP 2025 municipality boundaries (a pixel belongs to the municipality that holds its centre). Take the area-weighted mean class of the classified pixels, round half up. |
| Official or ours | The five classes are official. The mean and the rounding are **our interpretation**. |
| Ratings | 31 / 80 / 75 / 88 / 4 |
| Covers | 278 mainland municipalities. No ready-made per-municipality table of the chart was found, so the municipal value is computed from the national map. |

Limits: a static 2020 baseline, later fires and land-cover change are not in it. Averaging squeezes the top: only four municipalities reach 5. In dense cities only the small vegetated part counts (in Lisboa 16% of the area is classified). A daily fire-weather index per municipality also exists (IPMA). It changes every day and is **not** used.

### Flood (APA)

| | |
|---|---|
| Source id | `pt-apa-pgri2-flood-hazard-t100` |
| Dataset | Flood hazard maps of the 2nd-cycle flood risk management plans (PGRI 2022-2027) of APA (Agência Portuguesa do Ambiente), 100-year flood, inside the 63 mainland areas of significant flood risk (ARPSI) |
| Files | `https://sniambgeoviewer.apambiente.pt/Geodocs/shpzips/d312_apa_ar_2c_perigo_pc.zip`, listed at `https://dados.gov.pt/pt/datasets/carta-de-risco-de-inundacao-portugal-continental-perigosidade-da-inundacao-1`. Permanent water was taken out with `.../wise_surfacewaterbody_area_pt_vw_pub.zip`. |
| Licence | No restrictions stated (EU high-value dataset), credit APA |
| Native values | Hazard polygons in 5 classes per return period. Only the polygons of the 100-year flood are used, in any class. |
| Rule | Cut the union of the 100-year polygons with the CAOP 2025 boundaries, after removing reservoirs, estuaries and coastal water from both. Share = percent of the municipality's land in the 100-year flood zone (overlaps under 1 ha are ignored). Rating 1 = no ARPSI flood zone in the municipality, 2 = under 1%, 3 = 1% to under 5%, 4 = 5% to under 15%, 5 = 15% or more. |
| Official or ours | The ARPSI and their polygons are official. The thresholds are **our interpretation**. APA's own hazard classes are not used. |
| Ratings | 177 / 39 / 33 / 19 / 10 |
| Covers | 278 mainland municipalities. |

The issue text names the flood authority in a garbled way. It is read as APA, the national water authority that publishes the flood data of the EU Floods Directive.

**A rating of 1 does not mean "no flood hazard".** It means APA designated no significant flood-risk area there. ARPSI are mostly urban stretches chosen from past events and expected damage. Rural floodplains, small streams, flash floods and rain flooding in cities are not covered. Lisboa gets 2 although urban flooding is known there. The share also depends on the size of the municipality.

### Earthquake (NP EN 1998-1)

| | |
|---|---|
| Source id | `pt-np-en-1998-1-seismic-zones` |
| Dataset | Annex NA.I of the Portuguese National Annex of NP EN 1998-1 (Eurocode 8): every municipality with its seismic zone. IPQ (Instituto Português da Qualidade), technical committee CT 115, coordinated by LNEC. |
| Files | The IPQ standard is paid (`https://www.ipq.pt/loja/normas/norma/0a8a1405-05d2-ec11-a7b5-0022489d2016/`). The table was read from the CT 115 final-vote text of 2009-06-19 that OET (Ordem dos Engenheiros Técnicos) hosts at `https://www.oet.pt/downloads/eventos/EuroCodigo8.pdf`. |
| Licence | The standard is copyright of IPQ. Only the facts (zone per municipality, reference accelerations) are reused. No text is bundled. |
| Native values | Type 1 zone 1.1-1.6 with agR 2.5, 2.0, 1.5, 1.0, 0.6, 0.35 m/s². Type 2 zone 2.1-2.5 with agR 2.5, 2.0, 1.7, 1.1, 0.8 m/s². Madeira has Type 1 only, the Azores Type 2 only. |
| Rule | The governing value is the larger agR the annex defines for the municipality. agR 2.5 or more = 5, 2.0 or more = 4, 1.5 or more = 3, 1.0 or more = 2, below 1.0 = 1. |
| Official or ours | The zones and accelerations are official. The bands are **our interpretation**. |
| Ratings | 102 / 127 / 50 / 10 / 19 |
| Covers | All 308 municipalities. It is the only source that covers the islands. |

Checks: all 278 mainland codes match CAOP 2025. The zone table agrees with the hospital specification ACSS ET 05/2007 for 277 of 278 municipalities. A map of the published edition (Dlubal load-zone map, overlaid on CAOP) agrees for all 556 zone checks after one correction. The one correction is Óbidos, where the draft says Type 1 zone 1.5 and the other two sources say 1.4. It is applied, and its rating is 3 either way.

Limits: agR is the design acceleration on rock for a 475-year return period. It ignores soil amplification, liquefaction, tsunami and local geology, and it is one value for a whole municipality. Taking the larger of two actions with different spectral shapes is a simplification. The published 2010 edition and the amendment A1:2013 could not be read directly. The new EN 1998-1-1:2024 will bring a new national annex that may replace these values.

### Landslide (ELSUS v2, EU-level)

| | |
|---|---|
| Source id | `eu-jrc-elsus-v2-landslide-susceptibility` |
| Dataset | European Landslide Susceptibility Map ELSUS v2 of the European Commission JRC (European Soil Data Centre, ESDAC), Wilde et al. (2018), *Journal of Maps* |
| Files | ESDAC page `https://esdac.jrc.ec.europa.eu/content/european-landslide-susceptibility-map-elsus-v2` (download needs a registration form). The open copy used is the CC BY 4.0 deposit `https://doi.org/10.6084/m9.figshare.5878264.v3`, file `elsus_v2.asc`. |
| Licence | CC BY 4.0, attribution required |
| Native values | A 200 m grid with 5 classes: 1 very low, 2 low, 3 moderate, 4 moderate to high, 5 high to very high. |
| Rule | A cell belongs to the municipality that holds its centre. No-data cells are dropped. The rating is the area-weighted median class (the smallest class where the cumulative share reaches 50%). |
| Official or ours | The five classes are the source's own. The median is **our interpretation**. |
| Ratings | 108 / 64 / 91 / 15 / 0 |
| Covers | 278 mainland municipalities. ELSUS excludes Madeira and the Azores. |

**This is EU-level data, not a Portuguese national product.** The issue asks for EROS/CCDR data "when reachable as static datasets". No national landslide dataset could be downloaded:

- The civil protection (ANEPC) landslide layer of the 2019 national risk assessment is only served by a WFS server that refuses connections. All 22 ANEPC risk layers point to it.
- The 2023 national risk assessment (`https://prociv.gov.pt/media/h4fgmxul/anr2023_revis%C3%A3o_ultima.pdf`) has a landslide map (Fig. 28, IGOT-ULisboa) as an image only.
- The Reserva Ecológica Nacional (REN), which the regional coordination commissions (CCDR) approve, has an "unstable slopes" land type. The DGT WFS gives only one merged polygon per municipality, with no breakdown by type. This is the most likely meaning of "EROS/CCDR", but it is a guess. No landslide dataset called EROS was found.
- The SNIG catalogue has no mainland landslide data.

ELSUS is the best static dataset that could be downloaded and aggregated. Its rating-4 municipalities are mountain areas (Estrela/Lousã, Douro valley, Montemuro, Arouca), which fits the text of the 2023 national risk assessment.

Limits: a pan-European model for maps at 1:200,000 or coarser, not for local use. Its classes are relative inside each modelling region. The source rates its own reliability for the regions of the municipalities as good (9 municipalities), moderate (140), poor (71) or no information (58). Urban land lowers susceptibility, so Porto gets 1. The median hides steep minorities, so no municipality reaches 5. The `.prj` files shipped with ELSUS are wrong. The grid matches EPSG:3035.

### Coverage

| | Wildfire | Flood | Earthquake | Landslide |
|---|---|---|---|---|
| Mainland (278) | yes | yes | yes | yes |
| Madeira (11), Azores (19) | no | no | yes | no |

The Azores publish their own flood areas and landslide data through a regional government service. It was not ingested.

### How to read the ratings

- They are **coarse**. One number covers a whole municipality. Soil, slope at the plot, the distance to a river and the difference between a town centre and the countryside are not in it.
- They describe **hazard**, not risk. They say nothing about the building, its insurance or its value.
- A low rating can mean that the source has no signal. The flood rating 1 is the clearest case.
- The ratings of the four hazards come from four different methods. See the tables above.

## Updating a dataset

1. Take the new edition of the source. Use the CAOP edition that matches it, for the municipality list and the boundaries.
2. Compute the ratings with the rule in the tables above. If the rule changes, change this spec in the same commit.
3. Replace the JSON file. Update `dataset_version`, `retrieved_on`, `method` and `notes` of the source. Give a source a new `id` when it is a new edition. The sync then moves the ratings to it and deletes the old source.
4. If municipalities were merged or split, the keys change. Remove the old key and add the new one. The sync deletes the old region and its ratings.
5. Run `go test ./internal/hazard/`. It validates the file, checks the coverage rule and checks a few known answers. If a known answer moved on purpose, update the test.

The scripts that produced the first dataset are not in the repository. They were a GIS overlay of the national maps with the CAOP boundaries. This spec lists every source, layer, class and threshold they used.

## Not in this spec

- **The UI** (badges, sort and filter by risk, German copy): issue #45.
- **Finding the region of a Listing.** A Listing has a free-text `region`, a `country` and coordinates. Nothing maps them to a region key yet. The search endpoint lets a client look up a key by name. See [[../../ideas]].
- **More countries** (Sweden, Austria) and **Agent-suggested sources**: issue #46.
- **An Agent Tool or MCP tool** for hazard lookups: see [[../../ideas]].

## Verification

- `go test ./internal/hazard/` checks the shipped dataset (all 308 municipalities, the coverage rule, known answers) and every validation rule.
- With `TEST_DATABASE_URL` set, `go test ./internal/db/ ./internal/service/ ./internal/api/` runs the migration constraints, the sync (idempotent, prunes, leaves other countries alone) and both endpoints.
- By hand, after login: `curl -b cookies http://localhost:8080/api/hazards/regions/PT-1106` returns Lisboa with four ratings, and `.../regions?country=pt&q=lagoa` returns two regions.
