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

These are the same as in the predecessor project: UUID primary keys via `gen_random_uuid()`, `TIMESTAMPTZ` (UTC), hard deletes, `snake_case`, and singular table names.

This spec only lists **new tables** and **changed columns**. All other tables (`criterion`, `listing_criterion_grade`, `listing_note`, `app_config`, `task`) are carried over unchanged from the predecessor project.

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

A profile now owns exactly one `context` row (`scope = 'profile'`) and many `chat_message` rows.

New columns:

| Column | Type | Constraints | Description |
|---|---|---|---|
| `archived_at` | TIMESTAMPTZ | NULLABLE | When set, the profile is hidden from the default list. Archived profiles are shown in a separate "Archiv" section (means: "Archive"). NULL = active. |
| `ranking_summary` | TEXT | | Overall comparison summary of the profile's top listings. It is regenerated when the user asks (button or chat instruction). |

### `listing`

| Column              | Type             | Constraints | Description |
|---------------------|------------------|-------------|-------------|
| *(all predecessor project columns carried over unchanged)* | | | |
| `document_md`        | TEXT             |             | The Listing Document: original excerpt, translation, evaluation table, pros/cons, broker details. See [[../CONTEXT#Listing Document]] |
| `source_type`        | source_type      | NOT NULL, DEFAULT 'manual' | How this Listing entered the system |
| `source_url`         | TEXT             |             | Original URL the listing was ingested from. Used for deduplication ([[../01_mvp/01-03-listing-ingestion-via-chat#Deduplication]]). NULL for PDF/image/manual sources. |
| `source_file_path`   | TEXT             |             | Local path to the uploaded PDF, if `source_type = 'pdf'` |
| `user_confirmed_fields` | TEXT[]         | NOT NULL, DEFAULT '{}' | Names of fields the user edited by hand since the last Agent write — see [[../01_mvp/01-07-manual-review-and-edit]] |
| `title_image_id`    | UUID             | FK → listing_image, NULLABLE, ON DELETE SET NULL | Preview image shown in list/map views. The Agent sets it on the first image download. The user changes it with the star icon in the gallery UI ([[../../docs/adr/0012-image-storage-filesystem-media]]). |
| `needs_reevaluation` | BOOLEAN         | NOT NULL, DEFAULT false | Set to true when Criteria change or listing fields are edited by hand. Cleared after a successful `evaluate_listing`. It drives the ⚠️ icon in the list view and the bulk re-eval banner ([[../../docs/adr/0015-manual-bulk-reevaluation]]). |
| `kurzfazit`          | TEXT            |             | One-line summary of each Listing, written by the AI, for the Ranking view (for example "Beste Infrastruktur, aber kleinstes Grundstück", means: "Best infrastructure, but smallest plot"). Regenerated when the user asks.

`title_image_path`, `overall_score`, `status`, the seller fields, and `last_contact_at` are all carried over unchanged.

### `listing_image`

| Column       | Type        | Constraints             | Description |
|--------------|-------------|--------------------------|-------------|
| *(existing columns unchanged)* | | | |
| `file_path`  | TEXT        | NOT NULL                  | Path relative to `/data/`: `images/listings/{listing_id}/{uuid}.jpg`. The frontend serves it as `/media/{file_path}`. |
| `category`   | TEXT        |                          | Set by the Agent: `exterior` \| `interior` \| `floor_plan` \| `outbuilding` \| `other`. Nullable if it cannot be decided. |

---

## New Tables

### `context`

Holds the one Global Context and every Profile Context. See [[../CONTEXT#Context File]].

| Column        | Type          | Constraints                                  | Description |
|---------------|---------------|-----------------------------------------------|-------------|
| `id`          | UUID          | PK                                            | `DEFAULT gen_random_uuid()` |
| `scope`       | context_scope | NOT NULL                                      | `global` or `profile` |
| `profile_id`  | UUID          | FK → search_profile, NULL if `scope = 'global'` | The profile that owns this row. Required and unique when scope = profile |
| `content_md`  | TEXT          | NOT NULL, DEFAULT ''                          | The body of the Context File, in Markdown |
| `created_at`  | TIMESTAMPTZ   | NOT NULL                                       | |
| `updated_at`  | TIMESTAMPTZ   | NOT NULL                                       | |

**Constraints:**
- `CHECK ((scope = 'global' AND profile_id IS NULL) OR (scope = 'profile' AND profile_id IS NOT NULL))`
- Partial unique index: `UNIQUE (profile_id) WHERE scope = 'profile'` — exactly one Profile Context per profile
- Partial unique index: `UNIQUE ((true)) WHERE scope = 'global'` — exactly one Global Context row in the whole app, created on the first migration

### `context_revision`

This is an append-only audit log. A row is written **only when `context.content_md` really changes**. This happens on a direct user edit, or when the user confirms an Agent proposal ([[../../docs/adr/0009-proposal-pending-in-tool-calls]]). Proposals that are rejected or still pending do not create a `context_revision` row.

| Column        | Type        | Constraints              | Description |
|---------------|-------------|---------------------------|-------------|
| `id`          | UUID        | PK                        | `DEFAULT gen_random_uuid()` |
| `context_id`  | UUID        | FK → context, NOT NULL    | |
| `content_md`  | TEXT        | NOT NULL                  | Full snapshot from before the change. It is not a diff: the content is small, and making a diff is a job for display time. |
| `changed_by`  | TEXT        | NOT NULL                  | `user` or `agent` |
| `chat_message_id` | UUID    | FK → chat_message, NULL   | Set when `changed_by = 'agent'`. It links to the message that made the proposal. |
| `created_at`  | TIMESTAMPTZ | NOT NULL                  | |

### `chat_session`

One row per Search Profile.

| Column       | Type        | Constraints                                   | Description |
|--------------|-------------|-------------------------------------------------|-------------|
| `id`         | UUID        | PK                                              | `DEFAULT gen_random_uuid()` |
| `profile_id` | UUID        | FK → search_profile, NOT NULL                   | |
| `created_at` | TIMESTAMPTZ | NOT NULL                                         | |
| `updated_at` | TIMESTAMPTZ | NOT NULL                                         | Updated on every new message |
| `ended_at`   | TIMESTAMPTZ | NULLABLE                                          | Set when the user starts a new session. The old session then becomes read-only history ([[../01_mvp/01-01-chat-interface]]). NULL = active. |
| `token_count_estimate` | INTEGER | | Total token usage taken from API responses, updated after each turn. It is cached for the donut-chart display in the frontend ([[../../docs/adr/0014-context-window-token-display-manual-reset]]). |

**Constraints:**
- Partial unique index: `UNIQUE (profile_id) WHERE ended_at IS NULL` — at most one **active** session per profile. Ended sessions are kept as history.

### `chat_message`

| Column          | Type        | Constraints                       | Description |
|-----------------|-------------|-------------------------------------|-------------|
| `id`            | UUID        | PK                                  | `DEFAULT gen_random_uuid()` |
| `session_id`    | UUID        | FK → chat_session, NOT NULL         | |
| `role`          | chat_role   | NOT NULL                            | |
| `content`       | TEXT        | NOT NULL                            | Text prepared for the frontend: a user message, an Agent reply, or a short human-readable summary of a tool result. The LLM never sees this for `role = 'tool'` rows. It gets the structured `tool_result` instead. |
| `tool_calls`    | JSONB       |                                     | Raw tool-call requests, when `role = 'assistant'` and the turn used tools. It also holds pending proposals for `propose_criteria`/`update_context` ([[../../docs/adr/0009-proposal-pending-in-tool-calls]]). |
| `tool_result`   | JSONB       |                                     | Raw tool result, when `role = 'tool'`. Shape: `{"tool_call_id":"<id>","result":{...}}`. Used to replay the history to the LLM without loss ([[../../docs/adr/0010-tool-result-jsonb-for-replay]]). |
| `tool_name`     | TEXT        |                                     | Set when `role = 'tool'` |
| `attachment_path` | TEXT      |                                     | Uploaded file (PDF/image), when the user message had one |
| `listing_id`    | UUID        | FK → listing, NULL                  | Set when the tool calls of this turn created or updated a specific Listing. It lets the UI link from the chat straight to the Listing. |
| `created_at`    | TIMESTAMPTZ | NOT NULL                            | Sort key within the session |

---

## Foreign Key Delete Behaviour (new/changed only)

| FK | ON DELETE | Note |
|---|---|---|
| `context.profile_id` → `search_profile` | CASCADE | |
| `context_revision.context_id` → `context` | CASCADE | |
| `context_revision.chat_message_id` → `chat_message` | SET NULL | The history stays even if the message that caused it is deleted later |
| `chat_session.profile_id` → `search_profile` | CASCADE | |
| `chat_message.session_id` → `chat_session` | CASCADE | |
| `chat_message.listing_id` → `listing` | SET NULL | The chat history stays after a Listing is deleted |

**Listing delete order**: delete the files first (`os.RemoveAll("/data/images/listings/{id}")`), then the DB row. If deleting the files fails, the DB row is not deleted. This leaves no orphaned files and no orphaned DB references. `listing_image` rows are deleted by cascade through the FK after the `listing` row is deleted.

All FK behaviours from the predecessor project stay the same for the carried-over tables.

---

## Indexes (new only)

| Table              | Columns              | Reason |
|--------------------|-----------------------|--------|
| `chat_message`      | `session_id, created_at` | Load the history of a session in order |
| `chat_message`      | `listing_id`           | Jump from a Listing to the chat turn that created or updated it |
| `context_revision`  | `context_id, created_at` | Look up history, newest first |

---

## Open Questions / TBD

- Retention policy for `chat_message.attachment_path` (uploaded PDFs). Option 1: keep them forever, like the Listing Document, which is also permanent. Option 2: delete them after N days, once the Listing Document is written.
- Does `context_revision` need a size limit or a rollup, once Context Files have months of edits? Or will the volume stay so low (a few edits per week) that it never matters?
