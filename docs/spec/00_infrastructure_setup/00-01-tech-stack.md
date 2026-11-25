---
type: spec
created: 2026-07-28
title: Tech Stack & Project Structure
project: property-finder-obs-style
spec: tech-stack
phase: 0
order: 1
status: ready
updated: 2026-08-02
tags:
  - property-finder-obs-style
  - spec
  - tech-stack
  - go
  - vue
  - postgresql
  - claude
  - agent
  - phase-0
---

## Overview

This is a self-hosted client-server web app for one user. It has no authentication. It runs with Docker Compose. The backend, frontend, database, and deployment model are the same as in the predecessor project. This spec only describes what is **different**. The Chat/Agent layer replaces the old single-shot LLM-call layer and the custom scraper module.

---

## Technology Stack

| Layer               | Technology                              | Notes |
|---------------------|------------------------------------------|-------|
| Backend             | Go (Golang) + chi                       | Unchanged |
| Frontend            | Vue.js 3 (Composition API) + TypeScript | Unchanged |
| UI Components       | Naive UI                                | Unchanged |
| API                 | REST (JSON) + streaming Chat endpoint   | New: chat streaming, see below |
| Real-time Push      | SSE (Server-Sent Events)                | Used again for both background tasks and Chat token streaming |
| Database            | PostgreSQL                              | Unchanged, schema extended — see [[00-02-database-schema]] |
| Map                 | OpenStreetMap + OpenLayers              | Unchanged |
| AI                  | Claude API, agentic tool use + native PDF input | Claude is the default provider. LM Studio and OpenCode run behind the same provider interface through one OpenAI-compatible implementation (no native PDF/image input) — see [[00-03-llm-agent-integration#Provider Abstraction]] and [[../../docs/adr/0006-single-provider-for-agent-mvp]], [[../../docs/adr/0030-openai-compatible-agent-provider]] |
| Page fetching       | `net/http` GET + Readability-style text extraction | `chromedp` (headless Chrome) is reserved for after the MVP. We will add it when a site clearly needs JS rendering. The MVP uses plain HTTP. It is faster and needs no Chrome container. |
| Deployment          | Docker Compose                          | Two services: backend (with embedded frontend, serves `/media/` from the `/data/` volume) and PostgreSQL. No Chrome container (chromedp is deferred until after the MVP). |
| Lint / Format       | `golangci-lint` v2 (backend) · `oxlint` + `oxfmt` (frontend) | New: see [[#Code Quality — Lint & Format]] |

---

## What's Dropped vs. the Predecessor Project

| Dropped | Why |
|---|---|
| `internal/scraper/` field-extraction pipeline, HTML-reduction pipeline | The Agent reads the reduced page text directly and decides what matters. There is no custom per-field parser to maintain. |
| Per-portal scraper modules, portal anti-bot strategy as code | Anti-bot handling and "which link to follow" become Portal Rules written in plain language. They live in Context Files, and the Agent reads them at runtime — see [[../CONTEXT#Portal Rule]] |
| `profile_proposal` table / Proposal flow | Replaced by normal chat turns — see [[../CONTEXT#Explicitly dropped]] |
| LM Studio / OpenCode as day-1 providers | Reliable multi-turn tool use is a much higher bar for these than the old single-shot JSON calls. We will look at them again after the MVP — see [[../../docs/adr/0006-single-provider-for-agent-mvp]] |

What we **keep as-is**:
- The Criteria/Grade/Weight/Score data model and formula.
- The reason for dual scoring.
- The reason for SSE over WebSocket.
- SSE reconnect and resync.
- The patterns for robust LLM responses.

All of these still apply to how the Agent parses tool calls.

What we **keep in part**: task persistence and recovery applies only to the `search_portal` Tool ([[../../docs/adr/0028-portal-search-task-persistence]]), because a portal search is plain HTTP with no LLM cost. Agent chat turns use a simpler fire-and-forget model ([[../../docs/adr/0008-agent-turn-fire-and-forget]]). They do not use the `task` table.

---

## Backend — Go + chi

The reasons, the router, and the repository-pattern approach are the same as in the predecessor project. The new packages are:

```
backend/
├── internal/
│   ├── agent/                    # New: Chat/Agent loop
│   │   ├── loop.go               # Multi-turn tool-use orchestration (provider-independent)
│   │   ├── provider.go            # Provider interface + ChatEvent/ChatRequest types
│   │   ├── provider/              # Provider implementations
│   │   │   └── claude/            # Claude API provider (only MVP implementation)
│   │   ├── tools.go               # Tool definitions + dispatch registry
│   │   ├── context_builder.go    # Builds the system prompt from Global + Profile Context
│   │   └── session.go            # Chat Session persistence
│   ├── fetch/                    # New: page fetching, replaces internal/scraper/
│   │   └── fetch.go              # fetch_url Tool — HTTP GET + readability text extraction, returns text + image URLs + outbound links. chromedp deferred (see above).
│   ├── pdf/                      # New: PDF handling
│   │   └── pdf.go                # Passes uploaded PDFs to Claude as native document content blocks
│   ├── geocode/                   # New: address → coordinates
│   │   └── nominatim.go           # HTTP GET to nominatim.openstreetmap.org, rate-limited
│   ├── hazard/                    # New: static natural-hazard datasets (ADR 0025)
│   │   ├── hazard.go              # Loads and validates the dataset files embedded in the binary (go:embed)
│   │   └── datasets/pt.json       # One JSON file per country; HazardService syncs it into hazard_* at boot
│   ├── db/repository/
│   │   ├── chat_repo.go          # New
│   │   ├── context_repo.go       # New
│   │   ├── profile_repo.go       # Unchanged
│   │   ├── hazard_repo.go        # New (ADR 0025)
│   │   └── listing_repo.go       # Extended — document_md, source_type
│   └── ... (api/, events/, web/, config/ unchanged)
```

### Chat Streaming

`POST /api/profiles/{id}/chat/messages` accepts a new user message (text and an optional file upload). It starts the Agent loop. It streams the output in small parts over a separate SSE endpoint, through the `useChatStream` composable. Task SSE (`/api/events`) stays separate. It reports progress of background jobs (bulk re-evaluation, and Phase 3 later). There is no WebSocket, for the same reason as in the predecessor project.

### Static File Serving

Images and uploads live under `/data/` (a Docker volume). The Go backend serves them at `/media/` with `http.FileServer(http.Dir("/data"))`. The frontend refers to images as `/media/images/listings/{id}/{uuid}.jpg`. See [[../../docs/adr/0012-image-storage-filesystem-media]].

---

## Frontend — Vue.js 3 + TypeScript

The stack is the same as in the predecessor project, with these additions:

| Addition | Purpose |
|---|---|
| Chat view (`ChatView.vue`) | Shows the message list and an input with file drop for PDFs. It streams the Agent's reply and the tool-call progress ("Lese Seite…", "Folge Makler-Link…", "Extrahiere Bilder…") as they arrive. These progress texts mean: "Reading page…", "Following broker link…", "Extracting images…". |
| `ContextEditorView.vue` | Markdown editor and diff view for Global/Profile Context. The user edits directly with it, and also reviews additions the Agent proposes. |
| `useChatStream` composable | Reads SSE for chat-specific event types. It works like the existing task-event composable. |

Layout: the Chat view sits next to List/Ranking/Map for a Search Profile. It does not replace them — see [[../01_mvp/01-01-chat-interface]].

---

## Code Quality — Lint & Format

Both sides use one binary and a config file. No linter runs inside Docker. These are tools for the developer machine and CI.

| Side | Tool | Role | Config |
|---|---|---|---|
| Backend | `golangci-lint` v2 | Lint **and** format (`gofumpt` + `goimports` run as v2 *formatters*) | `backend/.golangci.yml` |
| Frontend | `oxlint` | Lint (correctness/suspicious/perf categories, `vue` + `typescript` plugins) | `.oxlintrc.json` (repo root) |
| Frontend | `oxfmt` | Format (Prettier-compatible, formats `.vue` SFCs, TS, JSON, CSS) | `.oxfmtrc.json` (repo root) |
| Frontend | `vue-tsc` | Type check — neither oxlint nor oxfmt knows types | `tsconfig.json` |

### Backend — golangci-lint

The config uses the **v2 schema** (`version: "2"`). In v2, formatters have their own `formatters:` block. They are not listed as linters. We enable the default set (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`). We add more linters that fit the shape of this app:

- **Many HTTP clients** (`fetch/`, `geocode/`, `agent/provider/claude/`): `bodyclose`, `noctx`, `contextcheck`
- **SQL/pgx**: `rowserrcheck`, `sqlclosecheck`
- **Error wrapping** (matters for robust LLM-response handling): `errorlint`, `errname`, `nilerr`, `nilnesserr`
- **Filesystem/`/data/` handling**: `gosec`
- **Quality**: `gocritic`, `modernize`, `revive`, `unparam`, `unconvert`, `wastedassign`, `misspell`, `nolintlint`

We **do not** enable these on purpose: `lll`, `funlen`, `mnd`, `varnamelen`, `wsl`, `nlreturn`, `exhaustruct`, `gochecknoglobals`, `wrapcheck`, `ireturn`. They are strict about style, they make a lot of noise, and they give little value for a project with one developer.

We do **not** set `goimports.local-prefixes` on purpose. Together with the import-grouping rule of `gofumpt`, it made the two formatters undo each other's work. Each `run` flagged again what the other formatter had just written, and this could not be fixed. Instead, local packages (module path `propertyfinder`) go into the same group as third-party imports.

```bash
golangci-lint run          # lint
golangci-lint fmt          # format
golangci-lint run --fix    # lint + autofix
```

Install: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` (or Homebrew).

### Frontend — oxlint + oxfmt

Both are Rust binaries from the [Oxc](https://oxc.rs) project. You install them as npm devDependencies. `oxfmt` replaces Prettier. Prettier configs are compatible, and `oxfmt --migrate prettier` exists. `oxlint` replaces ESLint for the rule categories it covers.

```bash
npm add -D oxlint oxfmt

npx oxlint          # lint
npx oxfmt           # format in place
npx oxfmt --check   # CI: fail on unformatted files
npx vue-tsc --noEmit  # type check
```

Both root configs have `ignorePatterns` that exclude `backend/`, `docs/`, `data/`, `dist/`, and `node_modules/`. Without this, oxfmt also reformats the Markdown specs and the backend YAML. Config discovery looks *up* the directory tree. So the root placement works if the frontend ends up at the repo root or in a `frontend/` subdirectory.

Known rough edge: `singleQuote` is not yet applied inside `.vue` script blocks ([oxc#17749](https://github.com/oxc-project/oxc/issues/17749)). This is only cosmetic. It does not affect correctness.

---

## AI — Claude API, Agentic Tool Use

Why Claude API is the only MVP provider:
- It has native multi-turn tool use with reliable JSON tool-call formatting.
- It understands PDF documents natively. You can send a PDF directly as a content block. No OCR step and no `pymupdf`-style extraction step is needed, unlike the reference Obsidian workflow.

Full detail: [[00-03-llm-agent-integration]].

---

## Deployment

Two containers and one volume:

```yaml
# docker-compose.yml
services:
  db:
    image: postgres:18
    environment:
      POSTGRES_DB: propertyfinder
      POSTGRES_USER: app
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      # Postgres 18's image expects the volume at /var/lib/postgresql (not
      # .../data) — it manages the version-specific data subdirectory itself.
      - pgdata:/var/lib/postgresql

  backend:
    build: ./backend
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: postgres://app:${DB_PASSWORD}@db:5432/propertyfinder
      CLAUDE_API_KEY: ${CLAUDE_API_KEY}
    volumes:
      - ./data:/data
    depends_on:
      - db

volumes:
  pgdata:
```

The frontend is embedded in the Go binary and served as static files. There is no nginx, no Chrome container, and no separate frontend service.

Deployment to a remote host is described here: [[../../deployment]]. In short: you copy a prebuilt image as a file, use `deploy/docker-compose.yml` with `image:` instead of `build:`, and use a named volume for `/data`.
