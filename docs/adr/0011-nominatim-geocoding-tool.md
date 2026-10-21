---
type: adr
title: "Geocoding with Nominatim (OpenStreetMap), added as the geocode Tool"
project: property-finder-obs-style
status: accepted
created: 2026-08-01
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. It affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] and [[../../spec/01_mvp/01-06-map-view]].

## Context

[[../../spec/01_mvp/01-06-map-view]] says: „Coordinates are populated by the Agent during ingestion (from address geocoding it performs as part of reading the Source).“ But the Tool catalogue in [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]] had no `geocode` tool. The Agent had no way to turn an address into Lat/Lon, unless the source page already had coordinates.

The question: which geocoding service do we use, and how does the Agent call it?

## Decision

**Use Nominatim (the OpenStreetMap geocoder) as the only geocoding backend for the MVP. Add it to the catalogue as a `geocode` Tool.**

### Why Nominatim

| Factor | Nominatim | Why it matters |
|---|---|---|
| No API key | No setup, no `app_config` entry, no account | Fits the app's goal of having no external dependencies |
| OSM data | Same data as the map view (OpenLayers) | No licence conflict. The app already shows the `© OpenStreetMap` credit |
| Coverage | Very good in Sweden, Portugal and Austria, the target regions | This is where the user actually searches |
| Rate limit | 1 request per second (fair-use policy) | One user, one listing at a time. This is more than enough |
| Cost | Free | No running cost for a personal tool |

The only real limit is 1 request per second. It matters for bulk re-geocoding (for example, re-evaluating 20 listings). That is not an MVP concern. A simple `time.Sleep(1s)` loop can handle it when needed.

### Alternatives considered

- **Google Geocoding API**: rejected. It needs an API key, a billing account and usage tracking. That is too much for a personal tool with one user. It also clashes with the OSM map licence: Google's terms say you must use Google Maps if you use their geocoding.
- **Mapbox Geocoding**: rejected. It needs an API key and an account. It has the same licence problem as Google.
- **Photon** (Komoot's geocoder, based on OSM): considered. It has the same data and the same free, no-key model. Its autocomplete and fuzzy matching are a bit better than Nominatim's. We rejected it only because Nominatim is the reference implementation and has more documentation. Photon can replace Nominatim directly if the 1 request per second limit ever becomes a problem.
- **The Agent reads coordinates only from the source page, with no geocoding tool**: rejected. Many listings in the reference regions (especially private sales and smaller brokers) show only a street address, without coordinates. The map view would be empty for these listings. That defeats its purpose.

## Consequences

### Tool Definition

This is a new entry in the Tool catalogue ([[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]]):

| Tool | Purpose | Backing implementation |
|---|---|---|
| `geocode` | Turn an address string into lat/lon coordinates using Nominatim | `internal/geocode` (HTTP GET to `nominatim.openstreetmap.org`) |

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

The Agent calls `geocode` during the ingestion flow ([[../../spec/01_mvp/01-03-listing-ingestion-via-chat#Flow — URL]]). It usually does this after `save_listing` (which puts the address into the listing) and before the final chat reply. The coordinates are then added to the listing with a second `save_listing` call or a dedicated field update.

If `geocode` fails, the Agent says so in the chat ("Adresse nicht gefunden, Inserat erscheint nicht auf der Karte", meaning: address not found, the listing will not appear on the map). The listing is saved without coordinates. This is the same behaviour as the "Coordinates cannot be determined" failure mode that is already specified.

### Backend Package

```
backend/internal/geocode/
└── nominatim.go    # HTTP GET, URL encoding, rate-limit guard (mutex + last-call timestamp)
```

About 40 lines. No external Go library is needed. It is a plain `net/http` GET to `https://nominatim.openstreetmap.org/search?q=<url-encoded>&format=json&limit=1`, with a proper `User-Agent` header (Nominatim's terms of use require it).

### Rate Limiting

A simple mutex-guarded last-call timestamp in the geocode package keeps 1 second between calls. The Agent loop already runs tool calls one after another, so the 1 request per second limit is kept naturally. The guard is only a second line of defence, in case the Agent calls `geocode` twice in quick succession. This is unlikely, but cheap to prevent.

### Config

Nominatim needs no configuration. The URL is hardcoded. If we later want a self-hosted Nominatim instance, we can add `app_config.geocoding.base_url` without a migration.
