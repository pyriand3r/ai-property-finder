---
type: guide
title: Operations Guide (Running the Server)
project: property-finder-obs-style
created: 2026-10-04
tags:
  - property-finder-obs-style
  - operations
  - mcp
---

# Operations Guide

This is a practical guide to running the backend (Go, embedded Vue SPA, Postgres 18). It also
explains how to use the MCP server. To build a image and copy it to a remote Docker host
without a registry, see [deployment.md](deployment.md). This document covers day-to-day
running. Both ways of running use the same configuration, backup, and MCP setup.

## 1. Variants

| Variant | Compose file | When |
|---|---|---|
| Local development/test | `docker-compose.yml` (repo root) | `build:` from source code, Postgres port published, `./data` bind mount |
| Production on a remote host | `deploy/docker-compose.yml` | `image:` from a prebuilt tarball, no Postgres port open to the outside, `/data` as a named volume — see [deployment.md](deployment.md) |

Both variants start and stop the same way (`docker compose up -d` / `down`). The sections
below apply to both, unless noted.

## 2. Start / Stop / Status

```bash
docker compose up -d --build       # local: builds and starts
docker compose up -d               # production host: starts with the loaded image
docker compose ps
docker compose logs -f backend
docker compose down                # stops, volumes are kept
curl localhost:8080/health         # {"status":"ok"} — the only endpoint without auth
```

The health check (`GET /health`) works without login on purpose. Both Compose files call it
with `wget` and no auth.

## 3. Configuration

There are two configuration levels (`backend/internal/config/config.go`):

### Tier 1 — Environment (`.env`, read only at start)

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `DATABASE_URL` | yes | – | Postgres connection string |
| `APP_PASSWORD` | yes | – | Login password for UI + API (ADR 0018) **and** Bearer token for `/mcp` (ADR 0019) |
| `DB_PASSWORD` | yes (Docker Compose only) | – | Postgres password. It only works at the **first setup** of the `pgdata` volume. After that it is ignored |
| `CLAUDE_API_KEY` | needed for the Agent | `""` | Written to `app_config` only once, at the very first boot (see Tier 2). After that the DB is the source of truth |
| `COOKIE_SECURE` | no | `true` | `Secure` flag of the session cookie. Use `false` only for local development without a TLS proxy in front |
| `HTTP_LISTEN_ADDR` | no | `:8080` | Bind address of the HTTP server |
| `DATA_DIR` | no | `/data` | Root folder for uploaded PDFs and listing images |

If `DATABASE_URL` or `APP_PASSWORD` is missing, the start fails (fail-fast, no silent partial
run). Without `CLAUDE_API_KEY` the backend still starts. The Agent then falls back to
`agent.NewUnconfiguredProvider()` and rejects every chat request with an error message. The
REST API, forms, and MCP tools that need no LLM call still work. These tools are `fetch_url`,
`download_image`, `geocode`, `list_profile_listings`, `compute_ranking`, `list_profiles`, and the
external evaluation tools `list_pending_evaluations`, `get_evaluation_brief`, and
`submit_evaluation` (an MCP client can grade a listing itself, ADR 0026).

### Tier 2 — `app_config` table (the DB is the source of truth after the first boot)

The LLM settings are `provider`, `base_url`, `model`, `timeout_seconds`, `max_retries`,
`max_tool_calls_per_turn`, `max_llm_roundtrips`, `reasoning_language`, and `api_key`. They are
stored as key-value rows (`key`, `category`, `value`) in `app_config`.

Edit them in the web UI: **Einstellungen** (`/settings`, link on the profile list). The same
change is possible with `PATCH /api/config`. The API key is write-only. No endpoint ever returns
it. `GET /api/config` only returns `api_key_configured: true/false`.

For **LM Studio** (local server) or **OpenCode**, choose the provider, enter the Base URL with the
version prefix (LM Studio: `http://localhost:1234/v1`, OpenCode Zen: `https://opencode.ai/zen/v1`) and
the model ID as the server names it. The API key is optional for LM Studio. These providers cannot
read PDFs or images; the Agent reports a linked PDF as an error. How well the Agent works depends on
the model's tool-use quality (docs/adr/0030). Inside Docker, `localhost` is the container, so use
`http://host.docker.internal:1234/v1` or the host's LAN address for LM Studio.

You do **not** need to restart the backend. A change applies to the next chat turn and the next
evaluation. A turn that is already running keeps its old settings (docs/adr/0022).

Direct SQL edits (for example `INSERT ... ON CONFLICT` on `app_config`) are **not** picked up by
the running process. Restart the backend after such an edit, or save once in the Settings view.

### Change the password

Change `APP_PASSWORD` in `.env`. Then run `docker compose up -d` (this only re-creates the
`backend` service). The JWT signing key comes from `APP_PASSWORD`. So this **immediately**
invalidates every existing browser session **and** every MCP Bearer token. This is wanted
behavior for a single-user system (ADR 0018). It is not a bug.

## 4. Backup

Two volumes hold user data:

```bash
# Database
docker compose exec db pg_dump -U app propertyfinder > backup.sql

# PDFs/images — local: just back up the bind-mount folder
tar czf data-backup.tar.gz ./data
# Production host (named volume appdata instead of bind mount):
docker run --rm -v <projekt>_appdata:/data -v "$PWD":/backup alpine \
  tar czf /backup/data-backup.tar.gz -C / data
```

Restore: load the database with `psql -U app -d propertyfinder < backup.sql` (into a freshly
migrated, empty DB). Unpack the data tarball back into the matching volume.

## 5. Migrations

Migrations run automatically at boot, before the HTTP listener starts. They fail fast
(`backend/cmd/server/main.go`) and only go forward (`golang-migrate/v4`,
`backend/internal/db/migrations/`). There is no separate migration CLI. A rollback to an old
image version only works if no new migration has run since.

## 6. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Login returns 204, then an instant redirect to `/login` | `COOKIE_SECURE=true` (default) in a deployment with no TLS proxy in front | Set `COOKIE_SECURE=false` (only for setups without an HTTPS proxy in front) |
| Backend container is `unhealthy` or restarts all the time | A migration fails, or `DATABASE_URL`/`APP_PASSWORD` is missing | Run `docker compose logs backend` — the error is in the first lines (fail-fast before the listener) |
| Chat answers with an error, but REST/forms work | `CLAUDE_API_KEY` was never set → `agent.NewUnconfiguredProvider()` | Set the API key under Einstellungen (`/settings`). `CLAUDE_API_KEY` in `.env` is only a one-time seed for the first boot |
| `POST /mcp` returns the SPA (`index.html`) instead of JSON-RPC | The image is **older than** ADR 0019 (no `/mcp` route) | Rebuild/load the image (`docker compose up -d --build`, or load the current `property-finder-image.tar.gz`) |
| `POST /mcp` returns `401` | The `Authorization: Bearer <APP_PASSWORD>` header is missing or wrong | Check the header. A `APP_PASSWORD` change invalidates old tokens immediately |

## 7. Using the MCP Server

The full internal Agent tool catalog is also available from outside through MCP (Streamable
HTTP transport, `github.com/modelcontextprotocol/go-sdk`). It is for external MCP-capable
clients like Claude Code, Claude Desktop, and opencode. They do not need to create a real chat
session (ADR 0019).

### Endpoint & Auth

- `POST /mcp` — Streamable HTTP, JSON-RPC 2.0. Answers come as a single JSON object
  (`JSONResponse: true`, no SSE streams; each tool gives one quick result).
- Auth: `Authorization: Bearer <APP_PASSWORD>`. This is the same value as the UI login
  password. There is no second secret (`RequireBearer` middleware,
  `backend/internal/api/auth.go`).
- The handler is **stateful**. The first answer (`initialize`) returns an `Mcp-Session-Id`
  response header. You must send this header with all later requests of the same session.

### Tool Catalog

`list_profiles` is the entry point. It returns the `profile_id` that every other tool needs as
a required field. Outside a chat session, there is no other way to get a profile UUID.

| Tool | Purpose | Key parameters |
|---|---|---|
| `list_profiles` | List all Search Profiles with UUID | – |
| `fetch_url` | Load a web page with GET. Return the readable main text, image URLs, and links | `profile_id`, `url` |
| `save_listing` | Create/update a listing | `profile_id`, `listing_id` (optional = update), `fields.*` (title, price, address, document_md, seller_*, …), `force_overwrite[]` |
| `download_image` | Load an image from a URL and save it with the listing (the first image automatically becomes the cover image) | `profile_id`, `listing_id`, `image_url`, `category` |
| `evaluate_listing` | Grade a listing against the profile criteria (grade A–F per criterion + overall Score) | `profile_id`, `listing_id` |
| `propose_criteria` | Create/change a criterion. It writes **at once**. There is no chat Confirm-Card, unlike the internal tool with the same name | `profile_id`, `action` (`add`/`edit`), `criterion_id` (for edit), `name`, `description`, `weight` (1–5), `is_subjective` |
| `update_context` | Append text to the Global or Profile Context. It also writes at once | `profile_id`, `scope` (`global`/`profile`), `operation` (only `append`), `content` |
| `geocode` | Turn an address into coordinates (Nominatim) | `profile_id`, `address`, `country` (optional, ISO-3166-1 alpha-2) |
| `list_profile_listings` | Overview of all listings in the profile (title, price, Score, status, source URL) | `profile_id` |
| `compute_ranking` | Ranking of all graded listings that are not rejected/archived, highest Score first | `profile_id` |
| `list_pending_evaluations` | List the listings of a profile that need a (new) evaluation (`needs_reevaluation = true`). No LLM key needed | `profile_id` |
| `get_evaluation_brief` | Get everything to grade one listing yourself: grading instruction, active criteria with IDs, the listing incl. document, both Context Files, user-confirmed fields, response format. No LLM key needed | `listing_id` |
| `submit_evaluation` | Send your grades for one listing. The server checks them (every active criterion exactly once, A–F, reasoning not empty), stores them, and computes the Score. No LLM key needed | `listing_id`, `grades[]` of `criterion_id`, `grade`, `reasoning` |

Over MCP, `propose_criteria` and `update_context` have **no undo**. The internal chat tools
with the same names (ADR 0004/0009) use a Confirm-Card. The MCP versions write directly with
no Confirm-Card. Changes are logged as `ChangedByUser`, not `ChangedByAgent`.

### Debugging with curl

For debugging, you can also walk through the handshake directly with `curl`. For everyday use,
use the client setups below.

```bash
TOKEN="<APP_PASSWORD>"
HOST="http://localhost:8080"

# 1. Initialize the session — note the response header "Mcp-Session-Id"
curl -sD - -o /dev/null -X POST "$HOST/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{
        "protocolVersion":"2025-06-18","capabilities":{},
        "clientInfo":{"name":"curl-test","version":"1.0"}}}'

SESSION="<from the Mcp-Session-Id header above>"

# 2. Confirm the initialized notification (no "id" = notification)
curl -s -X POST "$HOST/mcp" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" -H "Mcp-Session-Id: $SESSION" \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}'

# 3. Call a tool, for example list_profiles
curl -s -X POST "$HOST/mcp" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" -H "Mcp-Session-Id: $SESSION" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call",
       "params":{"name":"list_profiles","arguments":{}}}'
```

### Claude Code

```bash
claude mcp add --transport http property-finder https://<host>/mcp \
  --header "Authorization: Bearer <APP_PASSWORD>"
```

It connects directly. Claude Code supports static Bearer headers natively.

### opencode

In `opencode.json`/`opencode.jsonc`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "property-finder": {
      "type": "remote",
      "url": "https://<host>/mcp",
      "oauth": false,
      "headers": {
        "Authorization": "Bearer <APP_PASSWORD>"
      }
    }
  }
}
```

`oauth: false` is needed so opencode does not try to negotiate an OAuth flow with the server.
`/mcp` only knows the static Bearer token.

### Claude Desktop

The Custom Connector UI of Claude Desktop only accepts OAuth fields. It does not accept a
static Bearer token (ADR 0019, "Alternatives considered"). The workaround is the community
proxy `mcp-remote`. Claude Desktop can run it through its stdio server support:

```bash
npx mcp-remote https://<host>/mcp --header "Authorization: Bearer <APP_PASSWORD>"
```

Add it as a local stdio MCP server in the Claude Desktop configuration (Command: `npx`, Args:
`mcp-remote https://<host>/mcp --header "Authorization: Bearer <APP_PASSWORD>"`).

### Typical Workflow over MCP

1. `list_profiles` → get the `profile_id`.
2. `fetch_url` on the listing URL → main text/images/links.
3. `save_listing` with the extracted fields (for a new listing: leave out `listing_id`).
4. `download_image` for each relevant image.
5. `geocode` for the address (fills `latitude`/`longitude` for the map).
6. `evaluate_listing` → grade per criterion + Score.
7. `list_profile_listings` / `compute_ranking` for the overview/Ranking.

### Grading a Listing from the MCP Client (ADR 0026)

This works without `CLAUDE_API_KEY`. The client does the grading, the server only validates and
computes the Score.

1. `list_pending_evaluations` with the `profile_id` → listings that need a grade. (You can also
   take any `listing_id` from `list_profile_listings`.)
2. `get_evaluation_brief` with the `listing_id` → instruction, active criteria, listing, context.
3. Grade each active criterion by its `criterion_id`.
4. `submit_evaluation` with `listing_id` and one `{criterion_id, grade, reasoning}` per active
   criterion. A wrong set of criteria, a bad grade, or an empty reasoning is rejected and nothing
   is stored.
5. The Grades, the Score, and the Listing view in the UI show the result. The listing is no
   longer pending.
