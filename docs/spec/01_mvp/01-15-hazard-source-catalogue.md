---
type: spec
created: 2026-11-25
title: Hazard Source Catalogue and Agent Source Suggestions
project: property-finder-obs-style
spec: hazard-source-catalogue
phase: 1
order: 15
category: listings
status: ready
updated: 2026-11-25
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - hazard
---

## Overview

The ratings in [[01-13-hazard-ratings]] come from datasets that ship inside the backend. This spec adds two things around them (issue #46, decision [[../../adr/0027-hazard-source-catalogue-and-suggestions]]):

- a **source catalogue**: a declarative list of the hazard data sources known for each country, shipped or not;
- **source suggestions**: the Agent (chat or MCP) may propose a source that is not in the catalogue. The user approves or rejects it. Nothing is ingested without that approval.

Rules that stay from ADR 0025: no fetch at runtime, and no request writes the rating tables. **Approving a suggestion does not create ratings.** It clears the source for a release that ships its dataset. Terms: [[../CONTEXT#Hazard Source Catalogue]], [[../CONTEXT#Source Suggestion]].

## Source catalogue

One JSON file per country in `backend/internal/hazard/catalog/`, embedded in the binary. The files are `pt.json`, `se.json` and `at.json`.

```json
{
  "country": "AT",
  "entries": [
    {
      "id": "at-flood-hora",
      "hazard_type": "flood",
      "scope": "Österreich",
      "status": "candidate",
      "title": "HORA – Hochwasserrisikozonierung Austria",
      "publisher": "BMLUK (...)",
      "url": "https://hora.gv.at",
      "license": "",
      "notes": "Startpunkt, nicht geprüft: ..."
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `id` | Unique over all files. Starts with the lower-case country code and a dash (`at-`). |
| `hazard_type` | `wildfire`, `flood`, `earthquake` or `landslide`. |
| `scope` | The part of the country the source covers, in words (`Portugal (Festland)`, `Azoren`). Required. |
| `status` | `bundled` or `candidate`. |
| `source_id` | Only for `bundled`: the `id` of a source in the country's dataset file. |
| `title`, `publisher`, `url`, `license` | For `candidate`: title, publisher and a http(s) URL are required, the licence is optional (empty = not yet checked). For `bundled`: **not allowed**, they are copied from the dataset source. |
| `notes` | Required for `candidate`: what is known and what is still open (licence, format, how it maps to regions). |

`hazard.LoadCatalog` checks every file at boot, and a failure stops the backend, like `hazard.Parse` does for a dataset. It rejects:

- unknown fields, data after the object, a bad country code, an empty file;
- an id without the country prefix or used twice (also across files), an unknown hazard type or status, an empty scope;
- a `bundled` entry that names no source, an unknown source, a source of another hazard type or a source another entry already names, or that restates title, publisher, URL or licence;
- a dataset source that no `bundled` entry names (**every shipped source must be in the catalogue**);
- a `candidate` that names a source or lacks title, publisher, notes or a valid URL;
- two entries of one country with the same hazard type and URL (compared by `SourceURLKey`);
- a country that has a dataset but no catalogue file.

### Adding a source

- **A lead, not shipped:** add a `candidate` entry to the country's file. If the country has no file, create it (the file name is the lower-case country code). Run `go test ./internal/hazard/`.
- **Shipping a source:** follow "Updating a dataset" in [[01-13-hazard-ratings]]. Then add or change the entry to `bundled` with its `source_id` and delete the `candidate` text. If the user had approved a suggestion for it, the entry's URL is the same, so the suggestion cannot come back.

### Entries today

| Country | Hazard | Status | Source |
|---|---|---|---|
| PT | wildfire | bundled | ICNF (`pt-icnf-wildfire-hazard-2020-2030`) |
| PT | flood | bundled | APA (`pt-apa-pgri2-flood-hazard-t100`) |
| PT | earthquake | bundled | NP EN 1998-1 (`pt-np-en-1998-1-seismic-zones`) |
| PT | landslide | bundled | ELSUS v2 (`eu-jrc-elsus-v2-landslide-susceptibility`) |
| PT | flood, landslide | candidate | Azores regional government service (the idea "Hazard data for Madeira and the Azores" in [[../../ideas]]) |
| SE | flood | candidate | MSB |
| SE | landslide | candidate | SGU |
| AT | flood | candidate | HORA |
| AT | earthquake | candidate | GeoSphere Austria |

The Sweden and Austria entries are **starting points**. They name the publisher that is likely to hold the data and its site. The exact dataset, the licence, the format and how it maps to a region have **not** been checked. Their `notes` say so.

## Source suggestions

### Data

Table `hazard_source_suggestion` (migration `000006_hazard_source_suggestion`), columns in [[../00_infrastructure_setup/00-02-database-schema#Hazard tables]]. It is global: no profile, no listing. Unlike the `hazard_*` rating tables, requests write it.

| Status | Meaning |
|---|---|
| `proposed` | Suggested by the Agent or an MCP client. Waiting for the user. |
| `approved` | The user cleared it for ingestion. It is **not** ingested. |
| `rejected` | The user does not want it. The row stays so the source is not suggested again. |

A decision is final.

### The Tool `suggest_hazard_source`

Available to the chat Agent and, with the same arguments plus `profile_id`, on MCP (14 tools in total). It stores one `proposed` row.

| Argument | |
|---|---|
| `country` | Required. Two letters, any case. |
| `hazard_type` | Required. One of the four types. |
| `scope` | Optional, at most 200 characters. |
| `title`, `publisher` | Required, at most 200 characters each. |
| `url` | Required, http(s). |
| `license` | Optional. |
| `rationale` | Required, at most 2000 characters: why it is useful (coverage, resolution, format). |

The result is `{"status": "pending_confirmation", "suggestion_id": "...", "summary": "..."}`. An error result (the model can read it) means:

- invalid input (a 400 in REST terms);
- the source is in the catalogue: "Die Quelle steht schon im Katalog (`<entry id>`, Status `<status>`)." A source is the same when country, hazard type and `SourceURLKey` of the URL match, so `http://www.HORA.gv.at/` equals `https://hora.gv.at`;
- the source was suggested before: "Die Quelle wurde schon vorgeschlagen (Status `<status>`)."

The preamble of the Agent ([[../00_infrastructure_setup/00-03-llm-agent-integration]]) tells it to call the Tool only for a source it knows from a page it read, never to invent a URL, to tell the user that the decision is in the Global Context view, and not to ask again after a duplicate error. There is no Confirm-Card.

### API

All behind the login (ADR 0018).

| Endpoint | Answer |
|---|---|
| `GET /api/hazards/sources` | The catalogue, every entry in file order (files by country code), with the fields above plus `country`. For `bundled`, title, publisher, URL and licence come from the dataset source. |
| `GET /api/hazards/source-suggestions?status=` | Suggestions, newest first. `status` is optional: `proposed`, `approved` or `rejected`, anything else is a 400. |
| `POST /api/hazards/source-suggestions/{id}/approve` | The suggestion, now `approved`, with `decided_at`. 404 for an unknown id, 400 for a bad id, 409 if it is no longer `proposed`. |
| `POST /api/hazards/source-suggestions/{id}/reject` | Same, now `rejected`. |

There is no endpoint to create a suggestion from the UI (see [[../../ideas]]).

## UI

Section „Naturgefahren-Quellen“ below the editor on the Global Context view (`/context/global`, `HazardSourcesPanel.vue`).

- **Vorschläge des Agents**: open suggestions first, then decided ones. Each shows title, country, hazard, scope, publisher, licence, the link and the rationale. A `proposed` one has the buttons „Freigeben“ and „Ablehnen“. A decided one shows its status; an approved one says „Freigegeben – wartet auf Einbindung im nächsten Release“. A failed decision is a toast.
- **Quellenkatalog**: a table with country, hazard, scope, source (link, publisher, and the notes of a candidate) and a tag „Eingebunden“ or „Nicht eingebunden“.
- The intro text says that an approval changes no rating by itself.

## Not in this spec

- **Runtime ingestion.** Out of scope by ADR 0025/0027. Turning a source into ratings is the dataset update process in [[01-13-hazard-ratings]].
- **Real Swedish and Austrian datasets.** Only candidate entries exist.
- **Creating a suggestion by hand, linking an approved suggestion to the dataset that shipped it, an Agent Tool to read the catalogue:** see [[../../ideas]].

## Verification

- `go test ./internal/hazard/` validates the shipped catalogue against the datasets and every validation rule above.
- `go test ./internal/agent/` covers the Tool (arguments reach the service, service errors reach the model).
- With `TEST_DATABASE_URL` set, `go test ./internal/service/ ./internal/api/` runs the suggest and review workflow (duplicates, final decisions, 404/409) and the REST endpoints.
- `npm run test:run` covers `HazardSourcesPanel.test.ts` (approve, reject, error toast, catalogue tags).
- By hand, after login: `curl -b cookies http://localhost:8080/api/hazards/sources` lists the catalogue, and `/context/global` shows the section.
