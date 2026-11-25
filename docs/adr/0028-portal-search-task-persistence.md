---
type: adr
title: "Portal searches are persisted tasks that resume after a restart"
project: property-finder-obs-style
status: accepted
created: 2026-11-25
tags:
  - property-finder-obs-style
  - adr
  - agent
---

## Status

Accepted. Narrows [[0008-agent-turn-fire-and-forget]]: that ADR still holds for chat turns, but not for portal searches. Affects [[../../spec/01_mvp/01-16-portal-search]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] and [[../../spec/00_infrastructure_setup/00-01-tech-stack]]. Decided in issue #18.

## Context

The Agent can read one listing page at a time (`fetch_url`). Users also have a portal **search-result URL** (a list of many listings) and want the Agent to find the listings on it. Reading such a list takes several HTTP requests, one per result page, with a pause between them so the portal is not hit in a burst. It can run for a minute or more.

ADR 0008 chose "no recovery" for chat turns. Its reason was cost: a turn has already paid for its LLM calls, and a re-run after a restart would pay them again for no gain. That reason does not hold here. A portal search makes **no LLM call**. Its steps are plain HTTP GETs, so doing one again costs a few seconds and nothing else. The state worth keeping is small and plain data: the request, the next page, and the links found so far.

The `task` table already exists (migration 000001, with a `payload` JSONB column). The bulk re-evaluation uses it for progress only and does not resume.

## Decision

1. **`search_portal` is an Agent Tool** (and an MCP tool). `url` starts a search. `task_id` reads one back. It returns candidate listing links, split into `new_links` and `known_links` (a Listing of the profile already has that `source_url`). It does not create Listings. The Agent does that with `fetch_url` and `save_listing`, after the user agrees.
2. **A search is a `task` row** of type `portal.search`. `task.payload` holds everything needed to continue: the request, `next_page`, the links found, a recovery counter. The runner writes counters and payload together after every page (`TaskRepo.SaveProgress`).
3. **The search runs in a detached goroutine.** The Tool waits up to 30 seconds for it. If it is done, the result is returned. If not, `status = running` is returned and the Agent calls the Tool again with the `task_id`. The wait does not decide whether the search lives.
4. **After a restart, `PortalSearchService.Recover` continues every `running` portal search** at its saved page. It runs once at boot, after the migrations. A search is resumed at most 3 times. After that it is marked `failed`, so a search that crashes the process cannot restart forever.
5. **No scheduler.** Recurring and automatic search (Phase 3 in [[../../spec]]) stays out. This ADR only gives the Tool a durable task.

## Alternatives considered

- **Run the search inside the Tool call, with no task row.** Simple, but a search that is cut off by a restart is lost with no trace, and a slow search blocks the turn until the model's tool timeout. Rejected: the issue asks for persistence.
- **A generic recovery queue for all task types** (the predecessor's model: status, retries, `task_retry_at` index, polling worker). Rejected: only one task type needs it, and it would also have to decide what to do with bulk re-evaluation, which makes LLM calls and is excluded by the reasoning of ADR 0008. Recovery is a method of the service that owns the task type.
- **Resume bulk re-evaluation too.** Rejected for now: it spends LLM tokens per listing, and the user can start it again with one click. Not part of this issue.
- **Scrape listing cards (title, price) from the result page in Go.** Rejected: it needs per-portal code, which ADR 0003 rules out. Portal Rules in the Context Files name the pieces (`link_contains`, `page_param`); the Agent judges the rest.

## Consequences

- `task.type` gets a second value, `portal.search`. `domain.TaskTypePortalSearch`.
- `TaskRepo` gains `CreateWithPayload`, `SaveProgress`, `GetPayload`, `ListRunning`. `Create` is unchanged for callers.
- A server shutdown in the middle of a page leaves the task `running`. The next boot resumes it. A fetch that was in flight is repeated. That is safe: it is a GET.
- If two processes ran against one database, both would resume the same task. The app is single-user and single-instance (Docker Compose, one `backend` service), so no lock is taken.
- No SSE events are published for portal searches, and there is no UI for them. The Agent shows the result in the chat. If a task view is wanted later, the `task` rows and `task.updated_at` already carry what it needs.
