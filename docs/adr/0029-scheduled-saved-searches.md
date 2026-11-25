---
type: adr
title: "Saved searches run on a schedule; new links are handed to the Agent in chat"
project: property-finder-obs-style
status: accepted
created: 2026-11-25
tags:
  - property-finder-obs-style
  - adr
  - agent
---

## Status

Accepted. Builds on [[0028-portal-search-task-persistence]] (which left the scheduler out) and keeps [[0008-agent-turn-fire-and-forget]] for the Agent turn. Affects [[../../spec/01_mvp/01-17-scheduled-searches]]. Decided in issue #19.

## Context

The Tool `search_portal` finds the links on a portal search-result URL (ADR 0028). The user still has to ask for it each time. Phase 3 in [[../../spec]] wants the app to do this on its own: save a search URL, search it on a schedule, and bring the new listings in.

Two parts of that are different in cost. The search is plain HTTP and costs no LLM call. Importing a listing is an Agent turn (`fetch_url`, `save_listing`, `evaluate_listing`) and costs tokens. A scheduler that spends tokens by itself needs limits.

## Decision

1. **A saved search is a row in `saved_search`**: the `search_portal` request (`url`, `page_param`, `max_pages`, `link_contains`), a schedule, an import setting and the outcome of the last run. The schedule is "every N hours" (1–720). Not cron: one number is easy to explain in a form, and the search needs no wall-clock time.
2. **One scheduler goroutine** (`SavedSearchService.RunScheduler`, started in `main`) wakes every minute, takes the enabled searches whose `next_run_at` has passed, and runs them one after another. No new queue, no new task type: a run calls `PortalSearchService.Run`, which is the ADR 0028 task run to completion in the calling goroutine.
3. **A run is claimed in the database.** `UPDATE … SET last_status = 'running' WHERE last_status <> 'running'` decides who runs, so a "run now" click and the scheduler never run the same search twice. The same statement moves `next_run_at` one interval ahead, so a crash does not make the search run again at once.
4. **"New" means new since the last run.** The row keeps the `matchKey` of every link it already reported (`seen_links`, newest 5000). A link is new when it is not a Listing `source_url` of the profile (ADR 0028) and not in `seen_links`.
5. **Import goes through the chat.** When `import_new` is set, a run sends one user message, starting with `[Geplante Suche]`, to the profile's **active chat session** (`AgentService.SendToActiveSession`) and the normal Agent turn does the rest. The message lists at most `max_import` links (default 3, limit 10) and says the user agreed in advance, which the preamble understands (spec 00-04). Links beyond the limit stay unseen and are handed over at the next run. The limit is also tied to the turn budget of ADR 0013: 5 roundtrips cover about three listings when `fetch_url` calls run in one roundtrip.
6. **A busy chat is not an error.** If the session has a turn in progress, or the profile has no active session, nothing is sent and the links stay unseen. The next run tries again. A second turn must never run beside a running one in the same session.
7. **After a restart, interrupted runs are failed, not resumed.** `SavedSearchService.Recover` (at boot) sets `running` rows to `failed` ("Lauf durch Neustart unterbrochen"). Their portal task is continued by ADR 0028, but its links would not be handed to the Agent; the next scheduled run repeats the search. An Agent turn is not resumed either (ADR 0008).
8. **The UI is a tab "Suchen" per profile** with a list, a form, an on/off switch and "Jetzt suchen". The result of a run is also visible in the chat, because the import is a normal chat turn.

## Alternatives considered

- **Cron expressions.** More power, but a form field that users get wrong, and a parser to keep. Rejected; "every N hours" covers the use case (daily or twice a day).
- **Import without the chat (a private Agent session per run).** It would keep the user's chat clean. But then a run's Listings and tool cards need a second place in the UI, and `SendMessage` and the SSE stream already do exactly this. Rejected; the cost is one extra user message in the chat.
- **Import all new links with no limit.** A first run on a big result page would spend tokens for hundreds of listings and cannot finish within one turn. Rejected; a small limit per run plus the backlog in `seen_links` gives the same result over a few days.
- **Report only, never import.** Safe, but it is the manual workflow again. Kept as the setting `import_new = false`.
- **Use the `task` table for the schedule.** `task` rows are one run each. A schedule lives for months and holds user settings. Rejected; a run is still a `task` row (ADR 0028) and the saved search points to the last one (`last_task_id`).
- **Resume an interrupted run after a restart.** Needs to remember how far the import got. Rejected for now; a repeat is cheap because "seen" is only written when the links were handed over.

## Consequences

- Table `saved_search` (migration 000007). Routes under `/api/profiles/{id}/saved-searches`. No SSE events: the tab polls every 3 seconds while a search is running.
- Unattended LLM cost exists now, and is bounded: `max_import` listings per run, per search, at most once per interval. `import_new` can be turned off per search. The form says so.
- A link handed to the Agent counts as seen even if the Agent could not load it (a blocked page). It is not retried; the user can ask in chat.
- Single instance only (as in ADR 0028): two processes would both run the scheduler. The claim in item 3 would still keep a search from running twice at once.
- The Agent has no Tool to create saved searches and MCP has none either. Not part of this issue.
