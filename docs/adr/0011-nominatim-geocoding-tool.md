---
type: adr
title: "Geocoding via Nominatim (OpenStreetMap), added as geocode Tool"
project: property-finder-obs-style
status: accepted
created: 2026-08-01
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/01_mvp/01-06-map-view]].

## Context

[[../../spec/01_mvp/01-06-map-view]] states: „Coordinates are populated by the Agent during ingestion (from address geocoding it performs as part of reading the Source).“ But the Tool catalogue in [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]] had no `geocode` tool — the Agent had no mechanism to resolve an address to Lat/Lon unless the source page already contained coordinates.

The question: which geocoding service, and how does the Agent invoke it?

## Decision

**Nominatim (OpenStreetMap's geocoder) as the only geocoding backend for MVP. Added as a `geocode` Tool in the catalogue.**

### Why Nominatim

| Factor | Nominatim | Why it matters |
|---|---|---|
| No API key | Zero setup, no `app_config` entry, no account registration | Matches the app's no-external-dependency philosophy |
| OSM data | Same data source as the map view (OpenLayers) | No licence conflict — the app already carries `© OpenStreetMap` attribution |
| Coverage | Excellent in Sweden, Portugal, Austria — the target regions | Where the user actually searches |
| Rate limit | 1 req/s (fair-use policy) | Single-user, one listing at a time — more than sufficient |
| Cost | Free | No recurring cost for a personal tool |

The only practical limit is 1 request per second — this matters for bulk re-geocoding (re-evaluating 20 listings), but that is not an MVP concern and can be handled with a simple `time.Sleep(1s)` loop when needed.

### Alternatives considered

- **Google Geocoding API** — rejected: requires API key, billing account, and usage tracking. Overkill for a single-user personal tool. Also introduces a licence conflict with the OSM map (Google's ToS require using Google Maps if you use their geocoding).
- **Mapbox Geocoding** — rejected: requires API key and account. Same licence friction as Google.
- **Photon** (Komoot's geocoder, OSM-based) — considered: same data, same free/no-key model. Slightly better autocomplete and fuzzy matching than Nominatim. Rejected only because Nominatim is the reference implementation and more widely documented — Photon is a viable drop-in replacement if Nominatim's 1 req/s ever becomes a bottleneck.
- **Agent reads coordinates directly from the source page only, no geocoding tool** — rejected: many real-estate listings in the reference regions (especially private sales and smaller brokers) list only a street address without coordinates. The map view would be empty for these listings, defeating its purpose.

## Consequences

### Tool Definition

New entry in the Tool catalogue ([[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]]):

| Tool | Purpose | Backing implementation |
|---|---|---|
| `geocode` | Resolve an address string to lat/lon coordinates via Nominatim | `internal/geocode` (HTTP GET to `nominatim.openstreetmap.org`) |

Tool parameters:
```json
{
  "address": "string (required) — full address as known from the listing",
  "country": "string (optional) — ISO 3166-1 alpha-2, to bias results"
}
```

Returns:
```json
{
  "status": "ok",
  "lat": 55.6050,
  "lon": 13.0038,
  "display_name": "Kappmuren, Malmö, Sweden"
}
```

On failure (address not found, ambiguous, rate-limited):
```json
{
  "status": "error",
  "error": "address_not_found" | "ambiguous" | "rate_limited",
  "detail": "..."
}
```

### Agent Usage

The Agent calls `geocode` during the ingestion flow ([[../../spec/01_mvp/01-03-listing-ingestion-via-chat#Flow — URL]]), typically between `save_listing` (to get the address into the listing) and the final chat reply. The coordinates are then patched into the listing via a second `save_listing` call or a dedicated field update.

If `geocode` fails, the Agent reports it in the chat ("Adresse nicht gefunden, Inserat erscheint nicht auf der Karte") and the listing is saved without coordinates — same behaviour as the "Coordinates cannot be determined" failure mode already specified.

### Backend Package

```
backend/internal/geocode/
└── nominatim.go    # HTTP GET, URL encoding, rate-limit guard (mutex + last-call timestamp)
```

~40 lines. No external Go library needed — a plain `net/http` GET to `https://nominatim.openstreetmap.org/search?q=<url-encoded>&format=json&limit=1` with a proper `User-Agent` header (required by Nominatim ToS).

### Rate Limiting

A simple mutex-guarded last-call timestamp in the geocode package enforces 1-second spacing. The Agent loop dispatches tool calls sequentially anyway, so the 1 req/s limit is naturally respected — the guard exists only as defence-in-depth if the Agent calls `geocode` twice in rapid succession (unlikely but cheap to prevent).

### Config

Nominatim needs no configuration — the URL is hardcoded. If a self-hosted Nominatim instance is later desired, `app_config.geocoding.base_url` can be added without a migration.
