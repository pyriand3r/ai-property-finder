---
type: spec
created: 2026-11-25
title: Portal Search Tool and Task Persistence
project: property-finder-obs-style
spec: portal-search
phase: 1
order: 16
category: agent
status: ready
updated: 2026-11-25
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - agent
---

## Overview

A user often has a portal **search-result URL** (a page that lists many listings), not one listing. The Agent Tool `search_portal` reads such a page, or several result pages, and returns the links that may be listings (issue #18, decision [[../../adr/0028-portal-search-task-persistence]]). Terms: [[../CONTEXT#Portal Search]].

The Tool finds links only. It does **not** create Listings. The Agent opens each new link with `fetch_url` and saves it with `save_listing`, as in [[01-03-listing-ingestion-via-chat]], after the user said how many to load.

The Tool has no schedule of its own. Recurring runs of a saved URL are [[01-17-scheduled-searches]].

## The Tool `search_portal`

Available to the chat Agent and, with the same arguments plus `profile_id`, on MCP (15 tools in total).

| Argument | |
|---|---|
| `url` | Absolute http(s) URL of the search-result page. Starts a search. |
| `task_id` | UUID of a search from an earlier call. Reads it back. Use `url` **or** `task_id`. |
| `max_pages` | Optional, 1-5, default 1. |
| `page_param` | Name of the query parameter that holds the page number, for example `page`. Required when `max_pages` > 1. Page 1 is `url` unchanged. Page N is `url` with `page_param=N`. |
| `link_contains` | Optional. Keep only links that contain this text, for example `/bostad/`. |

A Portal Rule in a Context File ([[01-02-context-files]]) tells the Agent which `link_contains` and `page_param` a portal needs. There is no per-portal code (ADR 0003).

### Result

`PortalSearch` (`internal/domain/types.go`):

```json
{
  "task_id": "…", "status": "running | succeeded | failed",
  "url": "https://…", "pages_done": 2, "max_pages": 3,
  "new_links": ["…"], "known_links": ["…"],
  "stop_reason": "…", "error": "…"
}
```

- `known_links` are links equal to the `source_url` of a Listing in the profile. `new_links` are the rest. The split is made when the result is read, so a Listing saved since is reported as known. URLs are compared without fragment, trailing slash, `www.` and with sorted query keys.
- Collected links are: on the same host as `url` (ignoring `www.`), without fragment, not a result page of this search, containing `link_contains` when given, deduplicated in first-seen order, at most 300.
- `stop_reason` says why the search stopped early or found nothing: a page that failed to load, a PDF instead of a list, a page after the first without any new link (the portal repeats its last page: end of results), or no links at all (page rendered by JavaScript or blocked; there is no headless browser).
- The task is `failed` only when page 1 could not be read. A failure on a later page ends the search as `succeeded` with the pages read so far, and says so in `stop_reason`.
- An error result (the model can read it) means: no `url` and no `task_id`; a `url` that is not http(s); `max_pages` outside 1-5; `max_pages` > 1 without `page_param`; a `task_id` that is not a valid UUID or not a portal search of this profile.

The Tool waits up to 30 seconds for the search. A finished search returns at once. A slower one returns `status = running`, and the Agent calls the Tool again with the `task_id`. The preamble ([[../00_infrastructure_setup/00-04-system-prompt-preamble]]) tells it to.

## Task persistence

A search is a row in `task` (type `portal.search`, `profile_id`, `total` = `max_pages`, `done` = pages read). `task.payload` is the resumable state:

```json
{"url": "…", "page_param": "page", "link_contains": "/bostad/", "max_pages": 3,
 "next_page": 2, "recoveries": 0, "links": ["…"], "stop_reason": ""}
```

- The runner fetches one page at a time with a 1 second pause between pages. After each page it writes `done`, `failed_count` and `payload` in one statement.
- At boot, after the migrations, `PortalSearchService.Recover` (called from `cmd/server/main.go`) continues each `running` portal search at `next_page` with the links it had. A page that was in flight is fetched again.
- A search that was already resumed 3 times is set to `failed` ("Suche nach 3 Neustarts abgebrochen"). A payload that cannot be read is set to `failed` too.
- If the process shuts down while a page is loading, the runner leaves the task `running`. It does not mark it failed.
- Other task types are not resumed. Bulk re-evaluation stays as it is ([[01-04-criteria-and-scoring]]).

No SSE events and no UI for portal searches.

## Tests

- `internal/service/portal_search_service_test.go`: validation; link filtering; multi-page search that stops when a page repeats; failed first page; other profile's task is not found; resume at the saved page without refetching finished pages; give up after 3 resumes. The task rows need `TEST_DATABASE_URL`.
- `internal/agent/tools_test.go`: start with `url`, read back with `task_id`, error results.
