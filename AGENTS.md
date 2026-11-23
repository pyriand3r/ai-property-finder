# Repository Guidelines

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
| A bug found while working on something else (not what you were asked to fix) | `docs/buglist.md` — write it there **first**, before investigating or fixing, unless told to fix it now |
| A feature/improvement idea found while working on something else | `docs/ideas.md` — cross-reference every spec/ADR/layer/file the idea touches or depends on |
| Longer-form notes that fit no file above | new file under `docs/`, linked from `docs/00_readme.md` |

Rules:
- Update in place. Prefer editing the existing section over appending a duplicate one.
- Write docs in simple English: short sentences, common words, one idea per sentence. Keep real product
  text (German UI copy, system-prompt text) verbatim and add a short English gloss in nearby prose.
- Keep `AGENTS.md` short and factual — pointers to `docs/`, not copies of it. If a topic outgrows a few
  lines here, move it to `docs/` and leave a one-line pointer.
- Delete facts that become wrong. Stale docs are worse than missing docs.
- Docs are part of the change, not a follow-up. Write them in the same commit as the code they describe.

## Project Overview

Property Finder OBS-Style is a single-user, self-hosted web app that replaces a manual Obsidian-vault
workflow for cross-border real-estate hunting (Sweden/Portugal/Austria) with a chat-driven AI agent. A user
pastes a listing URL or uploads a PDF into a per-Search-Profile chat; a Claude-powered Agent (multi-turn
tool-use loop) fetches/reads the source, follows portal-specific secondary links, extracts photos, writes a
structured Listing Document (original text + German translation + evaluation), and grades the listing A–F
per user-defined weighted Criteria, producing a 0–100 Score. Two persistent, user-and-agent-editable
Markdown **Context Files** (Global + per-Profile — the app's own AGENT.md equivalent) steer the Agent
instead of hardcoded per-portal scraper code. Every Agent-written field is also visible and independently
editable in a plain form (ADR 0001) — the UI is never chat-only.

**Build state**: all 9 layers (0–8) of `docs/implementation-plan.md`, plus the interstitial Review-Fixes
(RF.1–RF.5) block, are checked off. The MVP is functionally complete per its own plan. Remaining work is
tracked as bugs in `docs/buglist.md` (5 open) and
ideas in `docs/ideas.md` (3 open). UI copy is German (`index.html` is `lang="de"`).

## Architecture & Data Flow

Two deployables: a Go backend (`backend/`, module `propertyfinder`, chi router) and a Vue 3 SPA (repo-root
`src/`) that Go embeds via `go:embed` (`backend/internal/web/web.go`, `dist/` populated by `npm run build`).
No ORM (`pgx/v5` + raw parameterized SQL); no message queue; SSE (not WebSocket) for all real-time push —
inherited from the predecessor project's decision (see `docs/00_readme.md`), not re-litigated here.

**Request lifecycle — sending a chat message** (`POST .../chat/sessions/{id}/messages`):
1. Frontend: `ChatInput.vue` emits `send` → `ChatView.vue` calls `useChat().send()` → `src/api/chat.ts`
   `sendMessage()` → `http.post()` (`src/api/http.ts`).
2. Backend: chi middleware (`RequestID` → `Logger` → `Recoverer` → `Timeout(60s)`, see below) →
   `ChatHandler.SendMessage` (`internal/api/chat.go`) → `parseUUID`/`decodeJSON` guards →
   `AgentService.SendMessage` (`internal/service/agent_service.go`).
3. `AgentService` persists the user's `chat_message` **synchronously** (survives a crash, ADR 0008), then
   spawns the actual agent turn in a **detached goroutine** using `context.Background()` — explicitly not
   the request context, so closing the tab doesn't kill an in-flight turn (`//nolint:contextcheck` with a
   rationale comment marks every such spot).
4. The turn assembles the system prompt (`internal/agent/context_builder.go`: preamble + Global Context +
   Profile Context + `user_confirmed_fields`, ADR 0016), then `internal/agent/loop.go`'s `Loop.Run` drives
   the roundtrip state machine against `agent.Provider` (max 5 roundtrips / 15 tool calls per turn per turn
   budget, ADR 0013), dispatching tool calls via `internal/agent/tools.go`'s `Registry` using
   `context.WithoutCancel(ctx)` so an in-flight tool call always finishes even after a Stop click.
5. Every `agent.ChatEvent` (text delta, tool call, done) is forwarded synchronously to `internal/events`'s
   `Broker`, which fans it out over `/api/events` (SSE, ring-buffered 200 events, 15s heartbeat, supports
   `Last-Event-ID` replay).
6. Frontend: `useChat.ts` already holds an open `EventSource('/api/events')` (opened `onMounted`), filters
   by `session_id`, accumulates streaming text/tool-progress, and on `chat.message` refetches the full
   message list (a multi-roundtrip turn can write several rows, so it's a refetch, not an append).
7. The endpoint itself returns **202 Accepted** immediately — the reply always arrives over SSE, never in
   the POST response (ADR 0008, fire-and-forget).

**Agent provider independence (ADR 0007)**: `internal/agent` defines its own wire-neutral `Provider`
interface, `Message`/`ContentBlock`/`ChatRequest`/`ChatEvent`/`Tool` types — zero Anthropic imports.
`internal/agent/provider/claude/client.go` is the only package that imports `anthropic-sdk-go`; it
translates to/from the neutral types using the SDK's streaming `ssestream` package. `internal/ai.Provider`
is a **separate**, single-shot interface (`EvaluateListing`, `GenerateRanking`, `AnalyzeRejection`) used
both directly by REST endpoints and wrapped as the `evaluate_listing` Tool — don't confuse the two
`Provider` interfaces.

**Config is two-tier** (`internal/config/config.go`): Tier 1 `Env` (env vars only: `DATABASE_URL`,
`HTTP_LISTEN_ADDR`, `CLAUDE_API_KEY`, `DATA_DIR`) fails fast on boot if `DATABASE_URL` is missing. Tier 2
`LLM` (provider/model/timeouts/roundtrip budgets) lives in the `app_config` DB table; `SeedFromEnv` writes
`CLAUDE_API_KEY` into it once on first boot, then the **DB is authoritative** — the env var is never
consulted again after that.

**Tier 2 is editable at runtime** (ADR 0022): `PATCH /api/config` (partial `{"llm": {...}}`, validated in
`internal/llm`, saved with `config.SaveLLM`) and the Settings view `/settings`. `internal/llm.Runtime` holds
an immutable `Snapshot` (config + `ai.Provider` + `agent.Provider`) behind an `atomic.Pointer`; a change
rebuilds and swaps it with no restart. `Runtime` is itself the `ai.Provider` handed to the evaluation/
ranking/rejection services. `AgentService.runTurn` takes **one** snapshot per turn — never read provider
and budget separately. The API key is write-only: no GET/PATCH response and no log line may contain it
(`config.LLM.String()` redacts it; keep it that way). The provider list and model suggestions come from
`llm.Providers()` — add a new provider there plus a branch in `llm.buildSnapshot`. Direct SQL edits of
`app_config` are not seen by the running process.

### Key Architecture Decisions (`docs/adr/`)

- **0001**: Chat is primary UI; plain form is mandatory fallback — not optional
- **0002**: Context Files are freeform Markdown, not structured settings
- **0003**: Portal rules as natural-language text in Context Files, not scraper code
- **0004**: One unified confirm-before-apply mechanism, no `Proposal` table
- **0005**: PDFs go to Claude as native document content blocks — no local text extraction
- **0006**: Claude API only for MVP; LM Studio/OpenCode deferred post-MVP
- **0007**: Agent loop is provider-independent (own `provider.go` interface, not the Anthropic SDK)
- **0008**: Agent turns are fire-and-forget — no recovery on server restart
- **0009**: Pending Agent proposals live in `chat_message.tool_calls` JSONB, confirmed via REST — no `Proposal` table
- **0010**: Tool results stored as JSONB in `chat_message` for lossless replay across providers
- **0011**: Address geocoding goes through OSM Nominatim via a dedicated `geocode` Tool (`internal/geocode/nominatim.go`, rate-limited)
- **0012**: Images on filesystem (`/data/images/listings/{id}/`), served at `/media/` via Go's `http.FileServer`
- **0013**: Agent turn budget is a fixed max-roundtrips count (5), not per-tool or token-based
- **0014**: No automatic context-window truncation — UI shows a token-fill donut, user manually starts a new session
- **0015**: Re-evaluation is manual, not automatic — user triggers per-listing or bulk from UI
- **0016**: Manually-edited `listing.user_confirmed_fields` are surfaced to the Agent every turn; `save_listing` skips them unless `force_overwrite` is set
- **0017**: Confirm-Card confirmed/rejected status is stored in the same `chat_message.tool_calls` JSONB (extends 0009) so it survives reload/tab-switch
- **0018**: Single password (APP_PASSWORD) gates Frontend+API access via an HS256 JWT in an httpOnly cookie; Secure flag toggled by COOKIE_SECURE for the documented behind-a-reverse-proxy deployments
- **0019**: MCP Streamable HTTP interface (`internal/mcpserver`) exposes the existing Agent Tool catalog
  plus `list_profiles` to external MCP clients, gated by a static Bearer token reusing APP_PASSWORD
- **0020**: Chat-uploaded JPG/PNG screenshots are sent to Claude as a native `image` content block, same
  pattern as ADR 0005's PDF path (`internal/attachment`, `Provider.SupportsImageInput()`)
- **0021**: Project is licensed AGPL-3.0-only (`LICENSE`); the network clause (§13) applies to modified hosted instances
- **0022**: LLM settings editable via `PATCH /api/config` + `/settings`; applied live through `internal/llm.Runtime` (atomic snapshot, API key write-only)
- **0023**: Replayed chat history goes through `agent.RepairToolPairing` (synthetic `tool_result` for a dangling `tool_use`, orphans dropped); `persistRoundtrip` writes each roundtrip in one transaction

0013/0014/0016/0017 together are the practical operating envelope for anyone touching `internal/agent/loop.go`.

## Key Directories

```
backend/
├── cmd/server/main.go        # entrypoint: env → DB pool → migrations → config seed →
│                              #   providers → api.NewServer → HTTP listen + graceful shutdown
└── internal/
    ├── api/          # chi handlers, one file per resource (chat, listings, profiles, criteria,
    │                 #   context_files, notes, images, upload, geocode, evaluate, ranking, tool_calls,
    │                 #   events, config_handler); router.go = composition root; helpers.go = shared
    │                 #   writeError/writeJSON/decodeJSON/parseUUID
    ├── service/      # business logic, one service per concern (agent, chat, listing, profile,
    │                 #   criterion, evaluation, ranking, rejection_analysis, image, note, context,
    │                 #   upload, tool_call)
    ├── domain/       # types.go — shared structs + domain error types (Validation/NotFound/Conflict/Upstream)
    ├── db/           # querier.go (pgx pool/tx abstraction), db.go, migrations/ (golang-migrate, forward-only)
    │   └── repository/  # one repo per entity (note, context, chat, image, listing, profile, task, criterion, grade)
    ├── agent/        # provider-independent agent loop: loop.go, tools.go (Registry/Dispatch),
    │                 #   provider.go (interface), context_builder.go (system prompt assembly),
    │                 #   unconfigured.go (no-API-key fallback)
    │   └── provider/claude/  # client.go — the ONLY package importing anthropic-sdk-go
    ├── llm/          # runtime.go (live config + provider snapshot, PATCH validation), providers.go (selectable
    │                 #   provider catalog), store.go (app_config persistence) — ADR 0022
    ├── ai/           # single-shot LLM ops: provider.go (interface), claude.go (impl), ops.go
    │                 #   (Evaluate/Rank/AnalyzeRejection), json.go (extractFirstJSON), retry.go, config.go
    ├── fetch/        # HTTP GET + readability text extraction (go-readability; no chromedp in MVP)
    ├── attachment/   # attachment.go — read/delete uploaded PDFs (ADR 0005) and images (ADR 0020) on disk (base64 in/out)
    ├── geocode/      # nominatim.go — Nominatim HTTP client, rate-limited
    ├── events/       # broker.go — single SSE Broker for chat streaming + background task progress
    ├── config/       # two-tier config loader (Env + DB-backed LLM config)
    └── web/          # web.go — go:embed of dist/, SPA-fallback handler; dist/ is gitignored except .gitkeep

src/                           # Vue 3 + TS SPA, repo root (Go embeds its build output)
├── api/            # one thin module per backend resource + http.ts (fetch wrapper, ApiError)
├── composables/     # useXxx() — the only state layer (no Pinia); one per feature/resource
├── types/          # interfaces mirroring backend/internal/domain structs 1:1 (snake_case fields)
├── utils/          # small pure helpers (chat.ts, listingStatus.ts, gradeColor.ts)
├── components/
│   ├── chat/        # ChatView, ChatMessageList, ChatInput, ChatConfirmCard, ChatListingCard, TokenDonut
│   ├── listings/     # largest folder: ListingDetailPanel, ListingForm, ListingGallery, ListingMap,
│   │                 #   ListingsMapPanel, ListingSplitPane, ListingsPanel, ListingListTable,
│   │                 #   ListingGradeGrid, ListingDocument, ListingNotes, RankingView
│   ├── context/      # ContextEditor (presentational), ProfileContextPanel (container)
│   ├── profile/      # ProfileCreateForm, ProfileListItem, ProfileTable
│   ├── settings/     # LlmConfigForm (presentational; used by SettingsView)
│   ├── criteria/     # CriteriaEditor, CriterionCreateForm, CriterionListItem
│   └── shared/       # ReevaluationBanner
├── views/          # route-level: ProfileListView, ProfileShellView (tab shell), ListingDetailView, GlobalContextView, SettingsView
└── router/index.ts  # all 6 routes, single source of routing truth

docs/
├── adr/            # numbered decision records + README.md index
├── spec/00_infrastructure_setup/, spec/01_mvp/   # feature/infra specs (see docs/00_readme.md for the map)
├── implementation-plan.md   # authoritative build-order/status (layers 0-8 + RF.1-5)
├── CONTEXT.md      # domain glossary — see Domain Vocabulary below
├── buglist.md      # open bugs found outside the current task (5 open)
└── ideas.md        # open improvement ideas (3 open)
```

No `scripts/` directory. CI is `.github/workflows/ci.yml` (see Testing & QA → CI).

## Development Commands

```bash
# Frontend (repo root)
npm run dev            # vite — dev server, proxies /api AND /media to http://localhost:8080
npm run build           # vue-tsc -b && vite build → dist/
npm run preview          # vite preview
npm run typecheck         # vue-tsc -b --noEmit  (project-reference build mode)
npm run lint            # oxlint
npm run format            # oxfmt (format in place)
npm run format:check       # oxfmt --check (CI gate, `.github/workflows/ci.yml`)

# Backend (from backend/)
go mod tidy
go build ./...
go test ./...
golangci-lint run           # lint — config backend/.golangci.yml, v2 schema
golangci-lint fmt           # format (gofumpt + goimports)
golangci-lint run --fix      # lint + autofix

# Docker Compose (repo root; v2 CLI, no hyphen — matches docker-compose.yml's own comments)
docker compose up -d --build       # backend on :8080, db (Postgres 18) on :5432
docker compose exec db psql -U app -d propertyfinder
docker compose logs -f backend
curl localhost:8080/health          # health check

# Remote Docker host (image as a file, no registry) — full workflow: docs/deployment.md
deploy/build-image.sh                  # → deploy/property-finder-image.tar.gz (linux/amd64; PLATFORM=linux/arm64 to override)
```

Env vars (`.env`, from `.env.example`): `DB_PASSWORD` and `APP_PASSWORD` (both required), `CLAUDE_API_KEY`
(required for the Agent; without it the backend falls back to `agent.NewUnconfiguredProvider()`), and
`COOKIE_SECURE` (optional, default true — set false only for a plain-HTTP local dev backend). Never commit
`.env` or `/data/` (Docker volume: uploaded PDFs and listing images).

## Code Conventions & Common Patterns

### Backend (Go)

- **Layering**: `api` (chi handler) → `service` (business logic) → `db/repository` (pgx). Pure constructor
  injection everywhere — `New*(deps...) *T` returning a struct pointer, no DI framework/container.
  `api.NewServer` in `router.go` is the single composition root.
- **Error handling**: domain error *types* (`*domain.ValidationError`, `*domain.NotFoundError`,
  `*domain.ConflictError`, `*domain.UpstreamError`) implement `error`; repositories translate
  `pgx.ErrNoRows` → `*domain.NotFoundError` at the repo boundary and wrap everything else with
  `fmt.Errorf("<op>: %w", err)`. The single `writeError()` in `internal/api/helpers.go` is the only place
  that maps these to HTTP status via `errors.As` (plus a special case for Postgres unique-violation →
  409); on an unmapped error it logs detail server-side and returns a generic "Internal server error" body
  — never leaks internals on 500. Sentinel errors elsewhere (e.g. `ai.ErrParse`) are asserted with
  `errors.Is`, not string matching.
- **Repository pattern**: small interface (e.g. `NoteRepository`) + concrete `*NoteRepo` backed by
  `db.Querier` — an interface over `*pgxpool.Pool` *or* a transaction, so the same repo works inside or
  outside a tx. Raw parameterized SQL, no ORM.
- **Handler pattern**: `parseUUID`/`decodeJSON` guard-clauses return early; success path calls a service
  with `request.Context()`, then `writeJSON`/`writeError`.
- **Async work**: fire-and-forget goroutines use `context.Background()` explicitly, not the request
  context (`//nolint:contextcheck` + rationale comment marks every instance) — work must outlive the HTTP
  request (ADR 0008). Cooperative cancellation goes through an explicit `sync.Mutex`-guarded
  `map[uuid.UUID]context.CancelFunc` (see `AgentService.turns`), not context cancellation propagation.
- **JSON tags**: snake_case everywhere on domain structs (`profile_id`, `is_subjective`, `created_at`);
  optional fields are pointer types with `omitempty`.
- Middleware chain (`router.go`): `RequestID` → `Logger` → `Recoverer` on *all* routes; a `Timeout(60s)`
  group wraps normal REST routes only — `GET /api/events` (SSE) and `/media/*` are mounted **outside** it
  since chi's `Timeout` wrapper doesn't implement `http.Flusher`. A `RequireAuth` group (docs/adr/0018)
  wraps everything except `/health`, `POST /api/auth/login`, and the embedded SPA static assets.

### Frontend (Vue 3 + TypeScript)

- **100% `<script setup lang="ts">`**, zero Options API. No Pinia/Vuex — state lives entirely in
  `src/composables/*.ts`, one per feature, each shaped the same way:
  ```ts
  export function useX(id: string) {
    const data = shallowRef<T[]>([]);
    const loading = shallowRef(false);
    const error = shallowRef<string | null>(null);
    async function fetchX() { /* try/catch/finally toggling loading/error */ }
    return { data: readonly(data), loading: readonly(loading), error: readonly(error), fetchX };
  }
  ```
  Components call the composable directly (usually in `onMounted`); every return value is `readonly()`-wrapped.
- **Props/emits use typed generics**, not the runtime `defineProps({...})` API:
  `defineProps<{ profileId: string }>()`, `defineEmits<{ save: [contentMd: string] }>()`.
- **API layer**: `src/api/http.ts` is the single fetch chokepoint (`http.get/post/patch/put/delete<T>()`);
  non-2xx throws `ApiError` (parsed from `{ error: string }`, mirrors backend's `apiError`); `204` →
  `undefined`. Components/composables never build URLs inline — always go through a named `src/api/*.ts`
  function.
- **Wire naming nuance**: object *field access* stays snake_case throughout the frontend, matching the
  wire shape 1:1 with no adapter/mapper layer (`listing.value.needs_reevaluation`,
  `statusDraft.rejection_comment` — see `src/types/*.ts`). Only *standalone local bindings* are camelCase
  (`const thumbnailUrl = ...`).
- **Two legitimate error-display patterns coexist** — don't try to unify them: composable-owned
  `error: Ref<string|null>` bound to `<NAlert v-if="error">` for the fetch lifecycle, vs. `useMessage()`
  (Naive UI toast) inside a component's own try/catch for one-off mutation actions.
- **Doc comments as living architecture notes**: non-trivial functions/types carry `/** ... */` blocks
  citing the exact backend file, ADR number, or spec section they implement (see `useChat.ts`). Follow
  this when adding non-obvious logic.
- **Lazy-loading**: heavy deps are code-split — `defineAsyncComponent` for `ListingsMapPanel.vue`
  (OpenLayers, ~300kB) in `ProfileShellView.vue`; all router routes are `() => import(...)`.
- **Naming**: PascalCase `.vue` files, camelCase `.ts`; composables are `useXxx` (filename = export name);
  API functions are verb-first (`listListings`, `createNote`); component event handlers are `handleXxx`;
  CSS classes are kebab-case prefixed with the component's own kebab-case name (BEM-lite,
  `<style scoped>` per component).
- Markdown rendering pipeline: `marked` → `DOMPurify` → `v-html` (never render unsanitized markdown).

## Important Files

- `backend/cmd/server/main.go` — full bootstrap sequence; read before changing startup behavior.
- `backend/internal/api/router.go` — composition root + full route table + middleware chain.
- `backend/internal/api/helpers.go` — shared `writeError`/`writeJSON`/`decodeJSON`/`parseUUID`.
- `backend/internal/domain/types.go` — canonical domain structs + error types shared across all layers.
- `backend/internal/agent/loop.go`, `tools.go`, `provider.go`, `context_builder.go`, `repair.go` — the agent loop; read
  ADR 0007/0013/0016 first.
- `backend/internal/config/config.go` — two-tier config loader.
- `backend/internal/web/web.go` — `go:embed` + SPA fallback; never delete `dist/.gitkeep`.
- `src/api/http.ts` — the fetch/error-handling chokepoint.
- `src/composables/useChat.ts` — most complex frontend file; canonical SSE + streaming-UI example.
- `src/types/*.ts` — source of truth for wire-shape contracts; keep in lockstep with `backend/internal/domain`.
- `src/router/index.ts`, `vite.config.ts` — routing and dev-proxy config (proxy target must match
  `docker-compose.yml`'s backend port).
- `.env.example`, `docker-compose.yml`, `backend/Dockerfile` — deployment/env contract.
- `backend/internal/mcpserver/` — MCP Streamable HTTP interface exposing the Agent Tool catalog to
  external clients (docs/adr/0019); `internal/service/tool_registry.go`'s `NewToolRegistry` is the shared
  Registry constructor both this and `AgentService.runTurn` use.

## Runtime/Tooling Preferences

| | |
|---|---|
| Backend | Go 1.26 (`backend/go.mod`), chi v5 router, `pgx/v5` (no ORM), `golang-migrate/v4` |
| AI SDK | `anthropic-sdk-go` v1.61.0 — imported **only** inside `internal/agent/provider/claude` |
| Frontend | Vue 3.5 (`<script setup>`), TypeScript 5.9 (strict), Vite 7, vue-router 4, Naive UI 2.42, OpenLayers (`ol`) 10, `marked` + `dompurify` |
| State mgmt | None (no Pinia/Vuex) — composables only |
| Package manager | npm (`package-lock.json` present; no yarn/pnpm lockfile) |
| Node/Go pins | Only pinned in `backend/Dockerfile`: `node:22-alpine` (frontend build stage), `golang:1.26-alpine` — no root `.nvmrc`/`.node-version` |
| Database | PostgreSQL 18 (`propertyfinder`, user `app`) |
| Deployment | Docker Compose, exactly 2 services (`backend`, `db`) — frontend has no separate container, it's embedded into the Go binary — both this repo's compose file and `deploy/docker-compose.yml` assume an external TLS-terminating reverse proxy in front (docs/adr/0018) |
| Lint/Format | `golangci-lint` v2 (backend) · `oxlint` + `oxfmt` (frontend) — both oxc configs ignore `backend/`, `docs/`, `data/`, `dist/`, `node_modules/`; `oxfmt` additionally ignores `.claude/`, `.idea/`, `*.md`; `oxlint` additionally ignores `*.d.ts` |

- `backend/go.mod` module path is `propertyfinder`.
- `.golangci.yml` deliberately does **not** set `goimports.local-prefixes`: combined with `gofumpt` (also
  enabled, for import grouping) it caused an unfixable oscillation between the two formatters — each `run`
  re-flagged what the other had just written. See the comment above the `formatters:` block in
  `backend/.golangci.yml`.
- Frontend configs (`.oxlintrc.json`, `.oxfmtrc.json`) live at the repo root on purpose: oxc config
  discovery walks *up* the tree, so they apply whether the frontend lands at the root or under `frontend/`.

## Testing & QA

### Backend (Go)

Stdlib `testing` only — **no testify/require/assert** in `go.mod`; assertions are plain
`if got != want { t.Errorf("got %v, want %v", got, want) }`. Tests are white-box (`_test.go` in the same
package), named `TestSubject_Scenario_ExpectedOutcome`. Non-obvious/regression tests carry a doc comment
above the function explaining *why*, often citing a spec section or a bug found in manual testing — follow
this style for new regression tests. No `testdata/` dir; fixtures are inline string/byte constants. No
`t.Parallel()` usage.

```bash
cd backend
go test ./...                          # all packages
go test ./internal/agent/...           # one package tree
go test -run TestLoop_Run_StopsAtRoundtripLimitWithPendingToolCall ./internal/agent/
```

Mocking, no library used, two conventions by collaborator kind:
1. **In-process** (repos/services): hand-written fakes implementing the exact narrow interface consumed
   (`fakeProvider`, `fakeListingWriter`, `fakeImageWriter`, `fakePageFetcher`, `fakeEvaluator`) — see
   `internal/agent/tools_test.go`'s `newTestRegistry(t, listings, images)` helper.
2. **Outbound HTTP** (Claude API, page/image fetch): `httptest.NewServer(http.HandlerFunc(...))` returning
   literal canned bytes — for Claude, a hand-authored raw SSE event stream as a string constant
   (`internal/agent/provider/claude/client_test.go`). No cassette/VCR library.

Covered packages: `internal/agent` (loop, tools, context builder), `internal/agent/provider/claude`,
`internal/ai` (JSON extraction, grade normalization), `internal/llm` (runtime swap/validation), `internal/api` (config handler only), `internal/service` (chat-history JSON merge logic),
`internal/attachment`, `internal/fetch`. **Not yet covered**: `internal/api` (all handlers except config), `internal/db`/repositories,
`internal/events`, `internal/geocode`, `internal/config`, `cmd/server`.

### Frontend (Vue + TypeScript)

**No test runner installed and no test files exist** (`src/**/*.test.ts`, `*.spec.ts`, `__tests__/` all
empty) — no vitest, `@vue/test-utils`, playwright, or cypress in `package.json`. This is a genuine gap, not
"configured but unused." The only automated frontend gates are `npm run typecheck` (`vue-tsc -b --noEmit`)
and `npm run lint` (`oxlint`). If adding frontend tests, you'll need to introduce the tooling first.

### CI

GitHub Actions, `.github/workflows/ci.yml`, runs on every push and pull request. Two jobs:
`backend` (`go build ./...`, `go test ./...`, `golangci-lint run` via golangci-lint-action, pinned to v2.15.0)
and `frontend` (`npm ci`, `npm run typecheck`, `npm run lint`, `npm run format:check`). Go version comes from
`backend/go.mod`; Node is 22, same as `backend/Dockerfile`. Run the same gates locally before opening a PR.
When you bump the golangci-lint version, change it in the workflow and fix any new findings in the same PR.

## Agent Pipeline (workflow rules)

This repo is developed through an agent pipeline: an orchestrator (Hermes) starts a coding agent (omp)
per issue, reviews the result, and merges it. The orchestrator works from GitHub issues — they are the
authoritative backlog. Rules that apply to every coding run:

- **Issue status labels** (exactly one `status/*` per issue): `status/new` (untriaged) →
  `status/ready` (startable) → `status/in-progress` (agent running) → `status/merge-request` (PR open).
  An agent may only start work on an issue labeled `status/ready`. The orchestrator manages the transitions.
- **Never commit or push to `main`.** Branch off `main`, name the branch `fix/#<n>`, `feature/#<n>` or
  `improvement/#<n>` after the issue, and open a pull request. Merging happens only via PR, done by the
  orchestrator after review — never by a local `git merge` into `main`.
- **Commit format:** `#<n> <short imperative English text>` (e.g. `#23 Edit LLM config via UI`). One
  issue, one branch, one PR. `Closes: #<n>` in the PR body; if the platform does not auto-close on
  merge, the orchestrator closes the issue manually.
- **Definition of done before opening the PR:** `cd backend && go build ./... && go test ./...`,
  `golangci-lint run` (config: `backend/.golangci.yml`), `npm run typecheck`, `npm run lint`,
  `npm run format:check` — all from a clean tree. Pre-existing failures (e.g. oxfmt drift in files the
  change does not touch) are noted in the PR body, not silently fixed.
- **Docs updated in the same commit** per Documentation Discipline above.

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
- Migrations: forward-only, `golang-migrate/v4`, run in-process on boot before the HTTP listener starts
  (fail-fast) — see `backend/internal/db/migrations/`
- DB password via `${DB_PASSWORD}` env var

## Domain Vocabulary

`docs/CONTEXT.md` is the authoritative glossary (with explicit "Avoid" aliases per term). Key terms:

- **Chat Session** — persistent, durably-stored message sequence scoped to one Search Profile.
- **Agent** — the LLM running a multi-turn tool-use loop inside a Chat Session (avoid "Assistant/bot").
- **Tool** — a typed function the Agent calls mid-turn: `fetch_url`, `save_listing`, `download_image`,
  `evaluate_listing`, `geocode`, `compute_ranking`, `read_pdf`, `propose_criteria`, `update_context`,
  `list_profile_listings`. The same catalog (plus `list_profiles` for discovery) is also reachable
  outside a chat session via the MCP interface at `POST /mcp` (docs/adr/0019).
- **Context File** — persistent Markdown steering Agent behavior (avoid "Prompt"). Two scopes: **Global**
  (app-wide) and **Profile** (per-Search-Profile; overrides Global).
- **Portal Rule** — a Context File *fragment* describing per-domain handling, not a structured entity.
- **Listing Document** — the Agent-authored write-up per listing, stored as Markdown in `listing.document_md`.
- **Ranking** — the curated, AI-narrated ordered view of active listings, regenerated only on request —
  distinct from the plain sortable List View.
- **Photo Set** — images per listing, tagged by category (exterior/interior/floor-plan/outbuilding/other).

## Deferred / Not in MVP

- `chromedp` (headless Chrome) — plain HTTP only for page fetching
- LM Studio / OpenCode providers — Claude API only (ADR 0006)
- Automated recurring search (Phase 3)
- `Proposal` table — replaced by chat confirmations (ADR 0004/0009)
- Per-portal scraper modules — replaced by Portal Rules in Context Files (ADR 0003)
