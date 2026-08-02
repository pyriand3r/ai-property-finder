---
type: spec
created: 2026-07-28
title: Tech Stack & Project Structure
project: property-finder-obs-style
spec: tech-stack
phase: 0
order: 1
status: ready
updated: 2026-07-28
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

Single-user, self-hosted client-server web app, no authentication, Docker Compose deployment. Backend, frontend, database, and deployment model are unchanged from [[../../../dev-property-finder/spec/00_infrastructure_setup/00-01-tech-stack|dev-property-finder]]. This spec only documents what's **different**: the Chat/Agent layer replaces the old single-shot LLM-call layer and the custom scraper module.

---

## Technology Stack

| Layer               | Technology                              | Notes |
|---------------------|------------------------------------------|-------|
| Backend             | Go (Golang) + chi                       | Unchanged |
| Frontend            | Vue.js 3 (Composition API) + TypeScript | Unchanged |
| UI Components       | Naive UI                                | Unchanged |
| API                 | REST (JSON) + streaming Chat endpoint   | New: chat streaming, see below |
| Real-time Push      | SSE (Server-Sent Events)                | Reused for both background tasks and Chat token streaming |
| Database            | PostgreSQL                              | Unchanged, schema extended — see [[00-02-database-schema]] |
| Map                 | OpenStreetMap + OpenLayers              | Unchanged |
| AI                  | Claude API, agentic tool use + native PDF input | Primary and only supported provider for MVP. LM Studio and OpenCode are planned swap targets behind the same provider interface, deferred post-MVP — see [[00-03-llm-agent-integration#Provider Abstraction]] and [[../../docs/adr/0006-single-provider-for-agent-mvp]] |
| Page fetching       | `net/http` GET + Readability-style text extraction | `chromedp` (headless Chrome) reserved for post-MVP when a site demonstrably needs JS rendering. MVP uses plain HTTP — faster, no Chrome container needed. |
| Deployment          | Docker Compose                          | Two services: backend (+ embedded frontend, serves `/media/` from `/data/` volume) + PostgreSQL. No Chrome container (chromedp deferred post-MVP). |

---

## What's Dropped vs. dev-property-finder

| Dropped | Why |
|---|---|
| `internal/scraper/` field-extraction pipeline, HTML-reduction pipeline ([[../../../dev-property-finder/docs/adr/0008-html-reduction-pipeline|ADR 0008]]) | The Agent reads reduced page text directly and decides what matters — no bespoke per-field parser to maintain |
| Per-portal scraper modules, portal anti-bot strategy as code ([[../../../dev-property-finder/docs/adr/0007-scraping-anti-bot-strategy|ADR 0007]]) | Anti-bot handling and "which link to follow" become natural-language Portal Rules living in Context Files, interpreted at runtime — see [[../CONTEXT#Portal Rule]] |
| `profile_proposal` table / Proposal flow ([[../../../dev-property-finder/docs/adr/0001-proposal-requires-user-confirmation|ADR 0001]]) | Superseded by ordinary chat turns — see [[../CONTEXT#Explicitly dropped]] |
| LM Studio / OpenCode as day-1 providers | Reliable multi-turn tool use is a materially higher bar for these than the old single-shot JSON calls; revisit post-MVP — see [[../../docs/adr/0006-single-provider-for-agent-mvp]] |

What's **kept as-is**: Criteria/Grade/Weight/Score data model and formula, dual-scoring rationale ([[../../../dev-property-finder/docs/adr/0002-dual-scoring-grade-and-score|ADR 0002]]), SSE-over-WebSocket rationale ([[../../../dev-property-finder/docs/adr/0003-sse-over-websocket|ADR 0003]]), SSE reconnect/resync ([[../../../dev-property-finder/docs/adr/0005-sse-reconnect-state-resync|ADR 0005]]), LLM response robustness patterns ([[../../../dev-property-finder/docs/adr/0006-llm-response-robustness|ADR 0006]]) — all still apply to the Agent's tool-call parsing.

What's **partially kept**: task persistence/recovery ([[../../../dev-property-finder/docs/adr/0004-task-persistence-and-recovery|ADR 0004]]) applies only to Phase 3 automated recurring search — Agent chat turns use a simpler fire-and-forget model ([[../../docs/adr/0008-agent-turn-fire-and-forget]]), no `task` table involvement.

---

## Backend — Go + chi

Same rationale, router, and repository-pattern approach as dev-property-finder. New packages:

```
backend/
├── internal/
│   ├── agent/                    # New: Chat/Agent loop
│   │   ├── loop.go               # Multi-turn tool-use orchestration (provider-independent)
│   │   ├── provider.go            # Provider interface + ChatEvent/ChatRequest types
│   │   ├── provider/              # Provider implementations
│   │   │   └── claude/            # Claude API provider (sole MVP implementation)
│   │   ├── tools.go               # Tool definitions + dispatch registry
│   │   ├── context_builder.go    # Assembles system prompt from Global + Profile Context
│   │   └── session.go            # Chat Session persistence
│   ├── fetch/                    # New: page fetching, replaces internal/scraper/
│   │   └── fetch.go              # fetch_url Tool — HTTP GET + readability text extraction, returns text + image URLs + outbound links. chromedp deferred (see above).
│   ├── pdf/                      # New: PDF handling
│   │   └── pdf.go                # Passes uploaded PDFs to Claude as native document content blocks
│   ├── geocode/                   # New: address → coordinates
│   │   └── nominatim.go           # HTTP GET to nominatim.openstreetmap.org, rate-limited
│   ├── db/repository/
│   │   ├── chat_repo.go          # New
│   │   ├── context_repo.go       # New
│   │   ├── profile_repo.go       # Unchanged
│   │   └── listing_repo.go       # Extended — document_md, source_type
│   └── ... (api/, events/, web/, config/ unchanged)
```

### Chat Streaming

`POST /api/profiles/{id}/chat/messages` accepts a new user message (text + optional file upload), starts the Agent loop, and streams incremental output over a dedicated SSE endpoint via `useChatStream` composable. Task SSE (`/api/events`) remains separate for background job progress (bulk re-evaluation, future Phase 3). No WebSocket — same rationale as [[../../../dev-property-finder/docs/adr/0003-sse-over-websocket|ADR 0003]].

### Static File Serving

Images and uploads live under `/data/` (Docker volume). The Go backend serves them at `/media/` via `http.FileServer(http.Dir("/data"))`. Frontend references images as `/media/images/listings/{id}/{uuid}.jpg`. See [[../../docs/adr/0012-image-storage-filesystem-media]].

---

## Frontend — Vue.js 3 + TypeScript

Same stack as dev-property-finder, plus:

| Addition | Purpose |
|---|---|
| Chat view (`ChatView.vue`) | Message list, input with file drop for PDFs, streaming render of the Agent's reply and tool-call progress ("Lese Seite…", "Folge Makler-Link…", "Extrahiere Bilder…") |
| `ContextEditorView.vue` | Markdown editor + diff view for Global/Profile Context, used both for direct user edits and for reviewing Agent-proposed additions |
| `useChatStream` composable | SSE consumption for chat-specific event types, analogous to the existing task-event composable |

Layout: Chat view sits alongside List/Ranking/Map for a Search Profile, not instead of them — see [[../01_mvp/01-01-chat-interface]].

---

## AI — Claude API, Agentic Tool Use

Rationale for choosing Claude API as the sole MVP provider: native multi-turn tool use with reliable JSON tool-call formatting, and native PDF document understanding (a PDF can be sent directly as a content block — no OCR or `pymupdf`-style extraction step needed, unlike the current manual Obsidian workflow). Full detail: [[00-03-llm-agent-integration]].

---

## Deployment

Two containers, one volume:

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
      - pgdata:/var/lib/postgresql/data

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

Frontend is embedded in the Go binary (served as static files). No nginx, no Chrome container, no separate frontend service.
