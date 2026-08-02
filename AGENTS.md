# AGENTS.md — Property Finder OBS-Style

## Documentation Discipline (read first)

`CLAUDE.md` is a pointer only — it contains `@AGENTS.md` and nothing else. Keep it that way. Never add
content to `CLAUDE.md`.

**Everything learned about this repository gets written down.** When you discover something non-obvious —
a build quirk, a gotcha, a convention, a decision, a corrected assumption — record it before moving on.
Where it goes:

| What you learned | Where it belongs |
|---|---|
| How to work in this repo: commands, structure, conventions, gotchas, stack facts | `AGENTS.md` (this file) |
| A decision with alternatives and consequences | new `docs/adr/NNNN-<slug>.md`, and index it in `docs/adr/README.md` |
| Feature/system behaviour, schemas, API shapes | the matching file under `docs/spec/` |
| A new term, or a renamed/retired one | `docs/CONTEXT.md` glossary |
| Build order or plan changes | `docs/implementation-plan.md` |
| Longer-form notes that fit no file above | new file under `docs/`, linked from `docs/00_readme.md` |

Rules:
- Update in place. Prefer editing the existing section over appending a duplicate one.
- Keep `AGENTS.md` short and factual — pointers to `docs/`, not copies of it. If a topic outgrows a few
  lines here, move it to `docs/` and leave a one-line pointer.
- Delete facts that become wrong. Stale docs are worse than missing docs.
- Docs are part of the change, not a follow-up. Write them in the same commit as the code they describe.

## Repo State

This is a **pre-implementation spec repository**. No code exists yet — only docs/ (specs, ADRs, implementation plan). The `implementation-plan.md` defines the exact build order in layers (0–8). Read it before writing any code.

## Tech Stack

| Layer | Tech |
|---|---|
| Backend | Go + chi router |
| Frontend | Vue 3 (Composition API, `<script setup>`) + TypeScript + Naive UI |
| Database | PostgreSQL 18 (`propertyfinder`, user `app`) |
| AI | Claude API (sole MVP provider per ADR 0006) |
| Real-time | SSE — no WebSocket (per dev-property-finder ADR 0003) |
| Map | OpenLayers + OSM |
| Deployment | Docker Compose: 2 services (backend + db), frontend embedded in Go binary |
| Lint/Format | `golangci-lint` v2 (backend) · `oxlint` + `oxfmt` (frontend) |

## Key Architecture Decisions (from docs/adr/)

- **0001**: Chat is primary UI; plain form is mandatory fallback — not optional
- **0002**: Context Files are freeform Markdown, not structured settings
- **0003**: Portal rules as natural-language text in Context Files, not scraper code
- **0004**: One unified confirm-before-apply mechanism, no Proposal table
- **0006**: Claude API only for MVP; LM Studio/OpenCode deferred post-MVP
- **0007**: Agent loop is provider-independent (own `provider.go` interface, not Anthropic SDK)
- **0008**: Agent turns are fire-and-forget — no recovery on server restart
- **0010**: Tool results stored as JSONB in `chat_message` for lossless replay across providers
- **0012**: Images on filesystem (`/data/images/listings/{id}/`), served at `/media/` via Go's `http.FileServer`
- **0015**: Re-evaluation is manual, not automatic — user triggers per-listing or bulk from UI

## Project Structure (once code exists)

```
backend/
├── cmd/server/main.go
├── internal/
│   ├── agent/          # Agent loop, provider interface, tools, context builder, session
│   ├── fetch/          # HTTP GET + readability text extraction (no chromedp in MVP)
│   ├── pdf/            # Pass PDFs to Claude as native document blocks
│   ├── geocode/        # Nominatim HTTP client, rate-limited
│   ├── db/             # Connection pool, migration runner, repositories
│   ├── api/            # chi handlers
│   ├── events/         # SSE plumbing (chat streaming + background task progress)
│   ├── config/         # Loads app_config from DB
│   └── web/            # Embedded frontend dist/
```

Frontend lives at repo root (`src/`, `vite.config.ts`, etc.) — Go embeds `dist/`.

## Dev Commands (once code exists)

```bash
docker-compose up                     # Backend on :8080, DB on :5432
psql -h localhost -U app -d propertyfinder  # Direct DB access
curl localhost:8080/health            # Health check
```

Backend: `go mod tidy`, `go build ./...`, `go test ./...`
Frontend: `npm run dev` (Vite dev server with proxy to :8080)

## Lint & Format

Config files already exist even though the code does not. Run these before considering any change done.

```bash
# Backend (from backend/) — config: backend/.golangci.yml, v2 schema
golangci-lint run           # lint
golangci-lint fmt           # format (gofumpt + goimports)
golangci-lint run --fix     # lint + autofix

# Frontend (from repo root) — configs: .oxlintrc.json, .oxfmtrc.json
npx oxlint                  # lint
npx oxfmt                   # format in place
npx oxfmt --check           # CI gate
npx vue-tsc --noEmit        # type check (oxlint is not type-aware)
```

- `backend/go.mod` uses module path `propertyfinder`.
- `.golangci.yml` deliberately does **not** set `goimports.local-prefixes`: combined with
  `gofumpt` (also enabled, for import grouping) it caused an unfixable oscillation between the
  two formatters — each `run` re-flagged what the other had just written. See the comment above
  the `formatters:` block in `backend/.golangci.yml`.
- Frontend configs live at the repo root on purpose: oxc config discovery walks *up* the tree, so
  they apply whether the frontend lands at the root or under `frontend/`.
- Both oxc configs ignore `backend/`, `docs/`, `data/`, `dist/`, `node_modules/`. Without that,
  `oxfmt` reformats the Markdown specs and `backend/.golangci.yml`.
- Rationale for the enabled/disabled linter sets: `docs/spec/00_infrastructure_setup/00-01-tech-stack.md`
  § Code Quality.

## Git Hygiene

`.gitignore` at repo root. Two non-obvious parts:

- `backend/internal/web/dist/` is gitignored **except** `.gitkeep`. `go:embed` fails at compile time
  if the target directory does not exist, so a fresh clone must still have the directory. Never
  delete that `.gitkeep`.
- The Node `dist` rules are anchored (`/dist/`, `/frontend/dist/`) rather than a bare `dist/`. A bare
  rule would exclude the directory `backend/internal/web/dist` outright, and git cannot re-include a
  file whose parent directory is excluded — the `.gitkeep` negation would silently stop working.

Never commit `.env` (holds `DB_PASSWORD` and `CLAUDE_API_KEY`) or `/data/` (Docker volume: uploaded
PDFs and listing images).

## Database

- Schema: `docs/spec/00_infrastructure_setup/00-02-database-schema.md`
- Migrations run in-app via `internal/db/` migration runner
- DB password via `${DB_PASSWORD}` env var

## Build Order

Follow `docs/implementation-plan.md` layer-by-layer (0 → 8). Each layer produces something manually testable with curl or browser. Dependencies diagram at bottom of that file.

## Domain Vocabulary

`docs/CONTEXT.md` defines all terms: Chat Session, Agent, Tool, Context File, Listing Document, Ranking, Photo Set, etc. Use these terms consistently — the glossary includes explicit "Avoid" aliases.

## Deferred / Not in MVP

- `chromedp` (headless Chrome) — plain HTTP only for page fetching
- LM Studio / OpenCode providers — Claude API only
- Automated recurring search (Phase 3)
- `Proposal` table — replaced by chat confirmations (ADR 0004)
- Per-portal scraper modules — replaced by Portal Rules in Context Files (ADR 0003)
