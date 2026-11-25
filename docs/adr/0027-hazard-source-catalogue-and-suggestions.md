---
type: adr
title: "Hazard sources are a declarative catalogue; the Agent only suggests new ones and the user approves before ingestion"
project: property-finder-obs-style
status: accepted
created: 2026-11-25
tags:
  - property-finder-obs-style
  - adr
  - hazard
---

## Status

Accepted. Extends [[0025-hazard-ratings-region-level-static-data]]. Affects [[../../spec/01_mvp/01-15-hazard-source-catalogue]], [[../../spec/00_infrastructure_setup/00-02-database-schema]] and [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]]. Decided in issue #46, the third sub-issue of #17.

## Context

ADR 0025 ships hazard ratings as static datasets, one JSON file per country, and only Portugal exists. Three things are missing:

1. A place that lists **which sources exist for which country**, including sources the app does not ship yet. Today the only list is the `sources` array inside a dataset, so a country with no dataset has no trace at all.
2. A way for the Agent to say "for Austria, HORA could be a flood source". The Agent often sees a listing in a country without hazard data.
3. A human check. A source decides what a buyer is told about the danger of a place. Nothing may enter the app because a model said so.

ADR 0025 also rules out fetching anything at runtime. So "ingestion" cannot mean "the Agent downloads a file and the app rates regions". Turning a raw map into a 1-5 rating per municipality is a GIS overlay with an interpretation step (spec 01-13, "Updating a dataset"). A model cannot do that reliably in a chat turn.

## Decision

1. **A declarative source catalogue per country.** One JSON file per country in `internal/hazard/catalog/` (embedded). Each entry names a hazard type, a scope (part of the country) and a status:
   - `bundled`: the data ships in the app. The entry names a source of the country's dataset by `source_id` and takes title, publisher, URL and licence from it, so they are stated once.
   - `candidate`: a known public source that is not ingested. It carries title, publisher, URL, optional licence and `notes` (what is still unknown).
   Adding a source or a candidate is a change to one file. No Go code changes.
2. **The catalogue is validated at boot.** `hazard.LoadCatalog` rejects a bad file and the backend does not start (same rule as the datasets). It also checks the catalogue against the datasets: a bundled entry must name an existing source of the same hazard type, and every dataset source must be named by exactly one bundled entry. A shipped source can therefore never be missing from the catalogue.
3. **The Agent suggests, it does not add.** The Tool `suggest_hazard_source` (and the same tool on MCP) stores a row in the new table `hazard_source_suggestion` with `status = 'proposed'`. This follows ADR 0024: an Agent or MCP client may only propose. A suggestion changes no rating and downloads nothing.
4. **Duplicates are refused.** A source is identified by country, hazard type and its normalised URL (`hazard.SourceURLKey`: no scheme, no `www.`, no fragment, no trailing slash). A source already in the catalogue, or suggested before in any status, is a 409 with a message the model can read. A **rejected** suggestion stays in the table so the Agent does not suggest the same source again.
5. **The user decides in the Global Context view.** `POST /api/hazards/source-suggestions/{id}/approve` and `.../reject`, a section "Naturgefahren-Quellen" on `/context/global`. A decision is final (a second one is a 409).
6. **Approval is the gate, not the ingestion.** `approved` means: the user cleared this source to be ingested. Ingestion itself stays what ADR 0025 says: a maintainer produces a dataset file following "Updating a dataset", adds the source and its `bundled` catalogue entry, and releases. The UI says so ("Freigegeben – wartet auf Einbindung im nächsten Release"). No runtime fetch is added.
7. **No Confirm-Card in the chat.** The suggestion is global, not tied to a profile, and the decision is not urgent. `suggest_hazard_source` returns `pending_confirmation` with a summary, and the Agent's preamble tells it to point the user to the Global Context view. A suggestion is not in `chat_message.tool_calls` (ADR 0009/0017 do not apply).

## Consequences

- Hazard coverage is visible per country: the catalogue shows what is shipped and what is only a lead. Sweden and Austria start with `candidate` entries only (spec 01-15). They are starting points and have not been checked for licence or format.
- Approval does not make a rating appear. A user may expect that. The copy in the UI and the spec state it. Automating ingestion would need a build tool that is not in the repository (see [[../../ideas]], "Keep the hazard dataset build scripts in the repository").
- An approved suggestion is not linked to the dataset that later ships it. The maintainer adds a catalogue entry (a `candidate` or `bundled` one) for it. The suggestion row stays as history. Because the catalogue entry has the same URL, a later suggestion of the same source is refused either way.
- The MCP tool count rises from 13 to 14.
- A user cannot create a suggestion by hand in the UI. The form-fallback rule (ADR 0001) is met for the part that matters, the review. A "propose a source" form is in [[../../ideas]].

## Alternatives considered

- **Let the user's approval trigger a runtime download and rating.** Rejected: it breaks ADR 0025 (no network at runtime, no writes to the hazard tables), and the 1-5 mapping is an interpretation that needs review per source.
- **Store candidates in the database, not in a file.** Rejected: the catalogue should be reviewed in a pull request and released with the datasets it describes.
- **Put suggestions into a Context File as a Portal Rule (ADR 0003).** Rejected: a Portal Rule steers how the Agent reads a portal. A hazard source is structured data with a status and a decision, and has to be queryable.
- **A Confirm-Card in the chat, stored in `chat_message.tool_calls`.** Rejected, see decision 7. It would also tie a global decision to one profile's chat history.
- **Hide suggestions once rejected.** Rejected: the row is what stops the Agent from repeating itself.
