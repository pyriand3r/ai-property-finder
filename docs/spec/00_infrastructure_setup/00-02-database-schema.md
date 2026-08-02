---
type: spec
created: 2026-07-28
title: Database Schema (PostgreSQL)
project: property-finder-obs-style
spec: database-schema
phase: 0
order: 2
status: ready
updated: 2026-08-02
tags:
  - property-finder-obs-style
  - spec
  - database
  - postgresql
  - schema
  - phase-0
---

## Conventions

Unchanged from [[../../../dev-property-finder/spec/00_infrastructure_setup/00-02-database-schema|dev-property-finder]]: UUID PKs via `gen_random_uuid()`, `TIMESTAMPTZ` (UTC), hard deletes, `snake_case`, singular table names.

This spec only lists **new tables** and **changed columns**. Everything else (`criterion`, `listing_criterion_grade`, `listing_note`, `app_config`, `task`) is carried over unchanged — see the dev-property-finder schema for full definitions.

---

## New Enum Types

**`chat_role`**
`user` | `assistant` | `tool`

**`context_scope`**
`global` | `profile`

**`source_type`**
`url` | `pdf` | `image` | `manual`

---

## Changed Tables

### `search_profile` — unchanged columns, new relationship

Now owns exactly one `context` row (`scope = 'profile'`) and many `chat_message` rows.

New column:

| Column | Type | Constraints | Description |
|---|---|---|---|
| `archived_at` | TIMESTAMPTZ | NULLABLE | When set, hides the profile from the default list. Archived profiles shown in a separate "Archiv" section. NULL = active. |
| `ranking_summary` | TEXT | | Overall comparison summary of the profile's top listings. Regenerated on user request (button or chat instruction). |

### `listing`

| Column              | Type             | Constraints | Description |
|---------------------|------------------|-------------|-------------|
| *(all dev-property-finder columns carried over unchanged — see there)* | | | |
| `document_md`        | TEXT             |             | The Listing Document — original excerpt, translation, evaluation table, pros/cons, broker details. See [[../CONTEXT#Listing Document]] |
| `source_type`        | source_type      | NOT NULL, DEFAULT 'manual' | How this Listing entered the system |
| `source_url`         | TEXT             |             | Original URL the listing was ingested from. Used for deduplication ([[../01_mvp/01-03-listing-ingestion-via-chat#Deduplication]]). NULL for PDF/image/manual sources. |
| `source_file_path`   | TEXT             |             | Local path to the uploaded PDF, if `source_type = 'pdf'` |
| `user_confirmed_fields` | TEXT[]         | NOT NULL, DEFAULT '{}' | Field names the user has manually edited since the last Agent write — see [[../01_mvp/01-07-manual-review-and-edit]] |
| `title_image_id`    | UUID             | FK → listing_image, NULLABLE, ON DELETE SET NULL | Preview image shown in list/map views. Agent sets on first image download; user changes via star icon in gallery UI ([[../../docs/adr/0012-image-storage-filesystem-media]]). |
| `needs_reevaluation` | BOOLEAN         | NOT NULL, DEFAULT false | Set true when Criteria change or listing fields are manually edited. Cleared on successful `evaluate_listing`. Drives the ⚠️ icon in list view and the bulk re-eval banner ([[../../docs/adr/0015-manual-bulk-reevaluation]]). |
| `kurzfazit`          | TEXT            |             | AI-written one-liner per Listing for the Ranking view (e.g. "Beste Infrastruktur, aber kleinstes Grundstück"). Regenerated on user request.

`title_image_path`, `overall_score`, `status`, seller fields, `last_contact_at` — all carried over unchanged.

### `listing_image`

| Column       | Type        | Constraints             | Description |
|--------------|-------------|--------------------------|-------------|
| *(existing columns unchanged)* | | | |
| `file_path`  | TEXT        | NOT NULL                  | Relative path from `/data/`: `images/listings/{listing_id}/{uuid}.jpg`. Served by frontend as `/media/{file_path}`. |
| `category`   | TEXT        |                          | Agent-assigned: `exterior` \| `interior` \| `floor_plan` \| `outbuilding` \| `other`; nullable if undetermined |

---

## New Tables

### `context`

Holds both the single Global Context and every Profile Context. See [[../CONTEXT#Context File]].

| Column        | Type          | Constraints                                  | Description |
|---------------|---------------|-----------------------------------------------|-------------|
| `id`          | UUID          | PK                                            | `DEFAULT gen_random_uuid()` |
| `scope`       | context_scope | NOT NULL                                      | `global` or `profile` |
| `profile_id`  | UUID          | FK → search_profile, NULL if `scope = 'global'` | Owning profile; required and unique when scope = profile |
| `content_md`  | TEXT          | NOT NULL, DEFAULT ''                          | The Context File body, Markdown |
| `created_at`  | TIMESTAMPTZ   | NOT NULL                                       | |
| `updated_at`  | TIMESTAMPTZ   | NOT NULL                                       | |

**Constraints:**
- `CHECK ((scope = 'global' AND profile_id IS NULL) OR (scope = 'profile' AND profile_id IS NOT NULL))`
- Partial unique index: `UNIQUE (profile_id) WHERE scope = 'profile'` — exactly one Profile Context per profile
- Partial unique index: `UNIQUE ((true)) WHERE scope = 'global'` — exactly one Global Context row app-wide, seeded on first migration

### `context_revision`

Append-only audit log. A row is written **only when `context.content_md` actually changes** — on direct user edit, or on Agent proposal confirm ([[../../docs/adr/0009-proposal-pending-in-tool-calls]]). Proposals that are rejected or still pending do not create a `context_revision` row.

| Column        | Type        | Constraints              | Description |
|---------------|-------------|---------------------------|-------------|
| `id`          | UUID        | PK                        | `DEFAULT gen_random_uuid()` |
| `context_id`  | UUID        | FK → context, NOT NULL    | |
| `content_md`  | TEXT        | NOT NULL                  | Full snapshot before the change (not a diff — content is small, diffing is a display-time concern) |
| `changed_by`  | TEXT        | NOT NULL                  | `user` or `agent` |
| `chat_message_id` | UUID    | FK → chat_message, NULL   | Set when `changed_by = 'agent'`, links to the proposing message |
| `created_at`  | TIMESTAMPTZ | NOT NULL                  | |

### `chat_session`

One row per Search Profile.

| Column       | Type        | Constraints                                   | Description |
|--------------|-------------|-------------------------------------------------|-------------|
| `id`         | UUID        | PK                                              | `DEFAULT gen_random_uuid()` |
| `profile_id` | UUID        | FK → search_profile, NOT NULL                   | |
| `created_at` | TIMESTAMPTZ | NOT NULL                                         | |
| `updated_at` | TIMESTAMPTZ | NOT NULL                                         | Bumped on every new message |
| `ended_at`   | TIMESTAMPTZ | NULLABLE                                          | Set when the user starts a new session — the old session becomes read-only history ([[../01_mvp/01-01-chat-interface]]). NULL = active. |
| `token_count_estimate` | INTEGER | | Cumulative token usage from API responses, updated after each turn. Cached for frontend donut-chart display ([[../../docs/adr/0014-context-window-token-display-manual-reset]]). |

**Constraints:**
- Partial unique index: `UNIQUE (profile_id) WHERE ended_at IS NULL` — at most one **active** session per profile; ended sessions are kept for history

### `chat_message`

| Column          | Type        | Constraints                       | Description |
|-----------------|-------------|-------------------------------------|-------------|
| `id`            | UUID        | PK                                  | `DEFAULT gen_random_uuid()` |
| `session_id`    | UUID        | FK → chat_session, NOT NULL         | |
| `role`          | chat_role   | NOT NULL                            | |
| `content`       | TEXT        | NOT NULL                            | Rendered text for the frontend (user message, Agent reply, or a human-readable summary of a tool result). The LLM never sees this for `role = 'tool'` rows — it receives the structured `tool_result` instead. |
| `tool_calls`    | JSONB       |                                     | Raw tool-call requests, when `role = 'assistant'` and the turn invoked tools. Also holds pending proposals for `propose_criteria`/`update_context` ([[../../docs/adr/0009-proposal-pending-in-tool-calls]]). |
| `tool_result`   | JSONB       |                                     | Raw tool result, when `role = 'tool'`. Shape: `{"tool_call_id":"<id>","result":{...}}`. Used for lossless history replay to the LLM ([[../../docs/adr/0010-tool-result-jsonb-for-replay]]). |
| `tool_name`     | TEXT        |                                     | Set when `role = 'tool'` |
| `attachment_path` | TEXT      |                                     | Uploaded file (PDF/image), when the user message carried one |
| `listing_id`    | UUID        | FK → listing, NULL                  | Set when this turn's tool calls created or updated a specific Listing — lets the UI deep-link from chat to the Listing |
| `created_at`    | TIMESTAMPTZ | NOT NULL                            | Ordering key within the session |

---

## Foreign Key Delete Behaviour (new/changed only)

| FK | ON DELETE | Note |
|---|---|---|
| `context.profile_id` → `search_profile` | CASCADE | |
| `context_revision.context_id` → `context` | CASCADE | |
| `context_revision.chat_message_id` → `chat_message` | SET NULL | History survives even if the originating message is later pruned |
| `chat_session.profile_id` → `search_profile` | CASCADE | |
| `chat_message.session_id` → `chat_session` | CASCADE | |
| `chat_message.listing_id` → `listing` | SET NULL | Chat history outlives a deleted Listing |

**Listing delete order**: files first (`os.RemoveAll("/data/images/listings/{id}")`), then DB row. If file deletion fails, the DB row is not deleted — no orphaned files, no orphaned DB references. `listing_image` rows cascade via FK after the `listing` row is deleted.

All dev-property-finder FK behaviours ([[../../../dev-property-finder/spec/00_infrastructure_setup/00-02-database-schema#Foreign Key Delete Behaviour|table]]) carry over unchanged for carried-over tables.

---

## Indexes (new only)

| Table              | Columns              | Reason |
|--------------------|-----------------------|--------|
| `chat_message`      | `session_id, created_at` | Load a session's history in order |
| `chat_message`      | `listing_id`           | Jump from a Listing to the chat turn that created/updated it |
| `context_revision`  | `context_id, created_at` | History lookup, newest first |

---

## Open Questions / TBD

- Retention policy for `chat_message.attachment_path` (uploaded PDFs) — keep indefinitely (mirrors the Listing Document's permanence) vs. prune after N days once the Listing Document is written
- Whether `context_revision` needs a size cap / rollup once Context Files see months of edits, or whether volume stays low enough (few edits/week) to never matter
