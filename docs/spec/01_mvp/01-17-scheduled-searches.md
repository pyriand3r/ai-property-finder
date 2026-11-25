---
type: spec
created: 2026-11-25
title: Scheduled Saved Searches
project: property-finder-obs-style
spec: scheduled-searches
phase: 3
order: 17
category: agent
status: ready
updated: 2026-11-25
tags:
  - property-finder-obs-style
  - spec
  - phase-3
  - agent
---

## Overview

A user saves a portal search-result URL, chooses how often it is searched, and the app brings the new listings in on its own (issue #19, decision [[../../adr/0029-scheduled-saved-searches]]). It builds on the Tool `search_portal` ([[01-16-portal-search]]). Terms: [[../CONTEXT#Saved Search]].

## Saved search

Table `saved_search` ([[../00_infrastructure_setup/00-02-database-schema]]). `SavedSearch` in `internal/domain/types.go`:

| Field | |
|---|---|
| `name` | Required, at most 100 characters, unique per profile (a duplicate is 409). |
| `url`, `page_param`, `max_pages`, `link_contains` | The request of `search_portal`, with its rules (absolute http(s) URL, 1-5 pages, `page_param` needed from 2 pages). |
| `interval_hours` | Schedule: a run every this many hours, 1-720. Default 24. |
| `enabled` | Default true. A disabled search never runs by itself; "Jetzt suchen" still works. |
| `next_run_at` | When the scheduler runs it next. Empty while disabled. An enabled search is first due at once (the next scheduler tick). |
| `import_new`, `max_import` | Whether a run hands new links to the Agent, and at most how many per run (1-10). Defaults true and 3. |
| `last_status` | `idle` (never ran), `running`, `succeeded`, `failed`. |
| `last_run_at`, `last_task_id`, `last_found`, `last_new`, `last_imported`, `last_summary`, `last_error` | Outcome of the last run. |

`seen_links` (the links earlier runs reported) is stored but not part of the JSON.

## REST

All under `/api/profiles/{profileId}/saved-searches`. A search of another profile is 404.

| Route | |
|---|---|
| `GET /` | The profile's saved searches, oldest first. |
| `POST /` | Create. Omitted fields get the defaults above. 201. |
| `PATCH /{searchId}` | Partial update. Enabling a disabled search schedules it for the next tick; disabling clears `next_run_at`; a new `interval_hours` of an enabled search applies from its next run. |
| `DELETE /{searchId}` | Delete. Listings it imported stay. 204. |
| `POST /{searchId}/run` | Start a run now, in the background. 202 with the search, `last_status = running`. 409 if it is already running. |

## A run

1. **Claim.** The search is set to `running` and `next_run_at` moves one interval ahead, in one statement. If another run holds the claim, nothing happens.
2. **Search.** `PortalSearchService.Run` runs the portal search to the end (task `portal.search`, [[01-16-portal-search]]). If page 1 could not be read, the run is `failed` with the reason in `last_error`; `seen_links` stays as it was.
3. **New links.** The links that are not a Listing `source_url` of the profile and not in `seen_links`. `last_found` is all candidate links, `last_new` the new ones.
4. **Import.** If `import_new` is set and there are new links, one user message goes to the profile's active chat session and starts an Agent turn. It starts with `[Geplante Suche]`, names the search, says the user approved the import, and lists the first `max_import` new links. The Agent loads them with `fetch_url`, saves with `save_listing`, grades with `evaluate_listing` and sums up in the chat ([[../00_infrastructure_setup/00-04-system-prompt-preamble]]). These links are added to `seen_links`; `last_imported` counts them. If the chat has a turn in progress, or the profile has no active session, nothing is sent: the summary says "Import übersprungen", and no link is marked seen.
5. **Report only.** With `import_new` off, all new links are added to `seen_links`, so the next run shows only what is new since.
6. **Outcome.** `last_status`, the counters and `last_summary` (for example "4 Treffer, 3 neu, 2 an den Agent übergeben") are written.

If the process stops during a run, the search stays `running`. At boot `SavedSearchService.Recover` sets it to `failed` ("Lauf durch Neustart unterbrochen"). The next scheduled run does the search again.

## Scheduler

`SavedSearchService.RunScheduler`, started in `cmd/server/main.go` after `Server.RecoverSavedSearches`. It checks every minute and runs due searches one after another. On shutdown its context ends: the run in progress stops and stays `running` (see above).

## UI

Tab **Suchen** in the profile (`/profiles/{id}/searches`): `SavedSearchesPanel.vue` lists the searches with schedule, next run, on/off switch, status of the last run with its summary or error, and the buttons "Jetzt suchen", "Bearbeiten" and "Löschen". "Suche speichern" opens `SavedSearchForm.vue`. The form says that automatic import uses LLM tokens. While a search is `running`, the list reloads every 3 seconds.

## Tests

- `internal/service/saved_search_service_test.go`: input validation; batching and the seen list across runs; busy chat keeps the links; report-only mode; a failed run keeps the seen list and stays scheduled; enable/disable/update schedule rules; a second run is refused and `Recover` settles a stuck run; `SendToActiveSession` does nothing for a busy or missing session. They use fakes for the portal search and the Agent and need `TEST_DATABASE_URL`, except the validation test.
- `internal/api/saved_searches_test.go`: defaults, 400/404/409, patch, delete (needs `TEST_DATABASE_URL`).
- `src/components/searches/SavedSearchesPanel.test.ts`, `src/utils/savedSearch.test.ts`.
