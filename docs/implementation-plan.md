---
type: plan
created: 2026-08-02
updated: 2026-08-02
title: Property Finder OBS-Style — Implementation Plan
project: property-finder-obs-style
tags:
  - property-finder-obs-style
  - implementation-plan
  - checklist
---

## Principle

Each step produces something you can **test by hand**: curl, browser, or a DB query. No step depends on a later step. After each step: test, adjust, and only then move on.

Layer 5 makes the app fully usable **before the agent** as a manual finder with AI scoring (form fallback, ADR 0001). After that, you can check every agent output from Layer 6 on in the real UI. You do not need DB queries for it.

---

## Layer 0: Skeleton

- [x] **0.1 — Docker Compose + DB**
  - `docker-compose.yml`: PostgreSQL 18 + Go backend (empty, health endpoint)
  - DB migration: create all tables from `00-02-database-schema.md`
  - Volume `pgdata`, volume `./data:/data`
  - **Test:** `docker-compose up` → `curl localhost:8080/health` → `200 OK`
  - **Test:** `psql -h localhost -U app -d propertyfinder -c "\dt"` → all tables are visible

- [x] **0.2 — Go project structure + config**
  - `backend/` with `go.mod` (module path: `propertyfinder`), `cmd/server/main.go`
  - Linting: `backend/.golangci.yml` already exists. `golangci-lint run` must pass cleanly from the first commit.
  - `internal/config/` — loads `app_config` from the DB
  - `internal/db/` — connection pool, migration runner
  - API router (chi) with `/health`
  - Global Context seed in the migration (default text from `01-02`)
  - **Test:** `curl localhost:8080/api/config` → returns the `llm.*` defaults
  - **Test:** `psql -c "SELECT content_md FROM context WHERE scope='global'"` → default text is there

---

## Layer 1: Profile + Context + Criteria (manual, no AI)

- [x] **1.1 — Search Profile CRUD**
  - Endpoints: `POST /api/profiles`, `GET /api/profiles`, `PATCH /api/profiles/{id}`, `DELETE /api/profiles/{id}`
  - `PATCH /api/profiles/{id}/archive` (sets `archived_at`)
  - On create: automatically create a `chat_session` and an empty Profile Context
  - **Test:** `curl -X POST .../api/profiles -d '{"name":"Schweden"}'` → `201` with an ID
  - **Test:** `curl .../api/profiles` → shows "Schweden"
  - **Test:** `curl -X PATCH .../api/profiles/{id}/archive` → `archived_at` is set in the DB

- [x] **1.2 — Context Files (edit by hand)**
  - Endpoints: `GET /api/context/global`, `PUT /api/context/global`, `GET /api/profiles/{id}/context`, `PUT /api/profiles/{id}/context`
  - Write a `context_revision` row on every `PUT` (changed_by = 'user')
  - `GET /api/profiles/{id}/context/history` → list of revisions
  - **Test:** `curl -X PUT .../api/profiles/{id}/context -d '{"content_md":"Bei Booli: Makler-Link folgen"}'` → `200`
  - **Test:** `curl .../api/profiles/{id}/context/history` → one revision is visible

- [x] **1.3 — Criterion CRUD (manual)**
  - Endpoints: `GET /api/profiles/{id}/criteria`, `POST /api/profiles/{id}/criteria`, `PATCH /api/criteria/{id}`, `DELETE /api/criteria/{id}`
  - Weight 1-5, with validation
  - No LLM call — fully manual
  - Hook in the write path (the listing table exists since 0.1, testable from 2.3): **a weight-only change** → pure-SQL Score recompute over all listings of the profile (no flag, no LLM). **Any other change** (name, description, objective/subjective, add, delete) → `needs_reevaluation = true` on all listings of the profile
  - **Test:** `curl -X POST .../api/profiles/{id}/criteria -d '{"name":"Lage","is_objective":false,"weight":4}'` → `201`
  - **Test:** `curl .../api/profiles/{id}/criteria` → shows "Lage", weight 4

---

## Layer 2: Listing + Evaluation (manual + single-shot LLM)

- [x] **2.1 — Listing CRUD (manual)**
  - Endpoints: `POST /api/profiles/{id}/listings`, `GET /api/profiles/{id}/listings`, `GET /api/listings/{id}`, `PATCH /api/listings/{id}`, `DELETE /api/listings/{id}`
  - All fields from the schema
  - `PATCH` on a structured field → `needs_reevaluation = true` **and** append the field name to `user_confirmed_fields` (needed for 8.2)
  - `DELETE` → first `os.RemoveAll("/data/images/listings/{id}")`, then the DB
  - **Test:** `curl -X POST .../api/profiles/{id}/listings -d '{"title":"Kappmuren 2","price":310000}'` → `201`
  - **Test:** `curl -X PATCH .../api/listings/{id} -d '{"price":305000}'` → DB: `needs_reevaluation = true`, `user_confirmed_fields = {price}`

- [x] **2.2 — evaluate_listing (single-shot LLM call)** — verified with one live Claude API call (`claude-sonnet-5`): real Grade A + German reasoning, `overall_score` computed, `listing_criterion_grade` row persisted
  - `POST /api/listings/{id}/evaluate` — no Agent, no tool use
  - Prompt: Criterion list + listing data → Grades with reasoning + Score
  - Provider interface: `Evaluate(ctx, criteria, listing) -> ([]Grade, score, error)`
  - `needs_reevaluation = false` on success
  - **Model:** Claude 3.5 Sonnet · GPT-4o-mini (single-shot, structured JSON. Haiku would be enough, but Sonnet is safer with edge cases)
  - **Test:** Create a Criterion (1.3), create a listing (2.1), `curl -X POST .../api/listings/{id}/evaluate` → DB: `overall_score > 0`, `listing_criterion_grade` rows exist

- [x] **2.3 — needs_reevaluation + bulk trigger**
  - `GET /api/profiles/{id}/listings?needs_reevaluation=true` → only stale listings
  - `POST /api/profiles/{id}/listings/reevaluate-all` → sequential goroutine, SSE stream (task SSE plumbing, reused in 6.3 for chat SSE)
  - **Test:** Change a Criterion description → `needs_reevaluation = true` on all listings → `curl .../reevaluate-all` → SSE stream shows progress → Scores are updated
  - **Test:** Change a Criterion weight → Scores are recomputed at once in SQL, flag stays `false` (no LLM call)

---

## Layer 3: File handling (images, PDF, geocoding)

- [x] **3.1 — Image upload + storage + serving**
  - `/media/` → `http.FileServer(http.Dir("/data"))` in the chi router
  - `POST /api/listings/{id}/images` — multipart upload, rename to a UUID
  - `GET /api/listings/{id}/images` — list of all `listing_image` rows
  - `DELETE /api/listings/{id}/images/{image_id}` — delete the file and the DB row
  - **Test:** `curl -F "file=@photo.jpg" .../api/listings/{id}/images` → `201` → `curl .../media/images/listings/{id}/{uuid}.jpg` → image is visible

- [x] **3.2 — Set/change the title image**
  - `PATCH /api/listings/{id}` with `{"title_image_id":"..."}`
  - On the first image upload: set `title_image_id` automatically
  - **Test:** Upload two images → `title_image_id` points to the first → switch to the second with PATCH

- [x] **3.3 — PDF upload**
  - `POST /api/upload/pdf` — multipart, max 32 MB, saves a temp file
  - **Test:** `curl -F "file=@listing.pdf" .../api/upload/pdf` → `201` with a path

- [x] **3.4 — Geocoding (Nominatim)**
  - `internal/geocode/nominatim.go` — HTTP GET
  - `GET /api/geocode?address=...&country=...` → `{"lat","lon","display_name"}`
  - Rate limiting: mutex + last-call timestamp
  - **Test:** `curl ".../api/geocode?address=Kappmuren+Malmo"` → coordinates

---

## Layer 4: Chat basics + frontend skeleton

- [x] **4.1 — Chat Session + message persistence**
  - Session CRUD, message CRUD (user messages only, no Agent yet)
  - `POST /api/profiles/{id}/chat/sessions/{sid}/messages` only saves
  - `POST /api/profiles/{id}/chat/sessions` creates a new session: the old session gets `ended_at` set (read-only), the new row is active
  - **Test:** Send a message → it is persisted → reload → it is still there
  - **Test:** Create a second session → the old one has `ended_at` set, both are in the DB

- [x] **4.2 — Frontend skeleton + profile list (Vue)**
  - Scaffolding: Vite + TypeScript + Vue Router + Naive UI. Go embeds `dist/` (single binary). Dev proxy to `:8080`.
  - `npm add -D oxlint oxfmt` — the configs (`.oxlintrc.json`, `.oxfmtrc.json`) are already in the repo root. Create the npm scripts `lint`/`format`/`typecheck`. **No** ESLint/Prettier.
  - `ProfileListView.vue` — list, archive, create/rename/archive
  - Click on a profile → profile shell with a tab strip (placeholder tabs)
  - **Test:** Browser → create a profile → click it → the profile shell appears

---

## Layer 5: Manual views (form fallback fully usable)

This makes the app usable before the Agent as a manual finder with AI scoring. It uses only the finished REST APIs from Layers 1-3.

- [x] **5.1 — Criterion Editor**
  - List of all Criteria with a weight slider, add/edit/delete
  - Weight change → Scores update at once (SQL recompute), no banner
  - Other changes → banner "N Inserate neu bewerten" + "Alle neu bewerten" button (2.3)
  - **Test:** Change a weight → new Scores, no banner → change a description → banner appears → click "Alle neu bewerten" → SSE progress

- [x] **5.2 — List View + Ranking View**
  - Sortable/filterable table (Score, price, size, date; filters status/Score/price)
  - Ranking: Score sorting + short summary (`kurzfazit`) + overall summary (`search_profile.ranking_summary`)
  - "Rangliste aktualisieren" button: isolated single-shot LLM call (no Agent, uses the provider seam from 2.2), saves `listing.kurzfazit` + `search_profile.ranking_summary`
  - **Model:** Claude 3.5 Sonnet · GPT-4o-mini (text generation, no tool use. Haiku is enough for short texts)
  - **Test:** Several listings → List View shows all → Ranking View sorts them → click "Rangliste aktualisieren" → short-summary texts + overall summary appear

- [x] **5.3 — Listing Detail View**
  - Form with all fields, rendered `document_md`, Grade grid
  - `needs_reevaluation` badge + "Neu bewerten" button (2.2)
  - Status management (rejected with a comment)
  - **Test:** Click a listing in the List View → Detail View → edit the price → `needs_reevaluation = true` AND `price` in `user_confirmed_fields` → "Neu bewerten" → badge is gone

---

## Layer 6: Chat UI + Agent integration (chat comes alive)

- [x] **6.1 — Frontend: Chat UI (Vue, without Agent)**
  - `ChatView.vue` — message list, input, file dropzone
  - Donut chart (shows `0%` until 6.3, because only the Agent fills `token_count_estimate`), "Neue Session" button (simple, with no warning and no disable. Those come in 8.3), session dropdown
  - Messages are persisted, with no replies (no Agent yet)
  - **Test:** Type a message → Enter → it appears in the list → reload → it is still there

- [x] **6.2 — Provider interface + Claude implementation**
  - `internal/agent/provider.go` — interface: `Chat()`, `FormatTools()`, `FormatMessages()`, `SupportsDocumentInput()`
  - `internal/agent/provider/claude/` — Anthropic client, SSE mapping
  - `ChatEvent` stream: `text_delta`, `tool_call_request`, `done`
  - **Model:** Claude 3.5 Sonnet · GPT-4o (tool-use reliability is critical here. There is no room for hallucinations in the JSON format)
  - **Test:** Unit test with a mock HTTP server → Claude tool call → `ChatEvent` is correct

- [x] **6.3 — Agent Loop (no Tools, echo only)**
  - `internal/agent/loop.go` — `Run()` as a goroutine
  - System prompt assembly (preamble + Global Context + Profile Context). The preamble has **no Tool section** while the Tool catalog is empty (otherwise the model hallucinates Tool calls as text)
  - SSE streaming of the Agent reply to the frontend
  - Fill `token_count_estimate` from the API usage (the donut has data from here on)
  - Limits: `max_llm_roundtrips`, `max_tool_calls_per_turn`
  - No real Tools yet. The Agent only answers with text
  - **Model:** Claude 3.5 Sonnet · GPT-4o (first real contact with the model. Here you see whether the prompt is right)
  - **Test:** Send a chat message → Agent answers (text only) → the reply streams into the chat

- [x] **6.4 — First Tools (fetch_url, save_listing, download_image)**
  - Tool registry + dispatch in `loop.go` (from here on the preamble has the full Tool section)
  - `fetch_url` — HTTP GET + Readability text extraction
  - `save_listing` — DB insert/update with `force_overwrite`
  - `download_image` — HTTP GET → `/data/images/...` → `listing_image` row
  - Inline Listing Card in the chat: `chat_message.listing_id` → card with title, price, thumbnail, Score. A click opens the Detail View (5.3)
  - The Agent can: URL → fetch → save → download images → reply
  - **Model:** Claude 3.5 Sonnet · GPT-4o (first multi-turn tool use. The model must read Tool results correctly and continue from them)
  - **Test:** Paste a URL into the chat → Agent loads the page, saves the listing, loads the images → listing is visible in the List View (5.2), images in the Detail View, card in the chat is linked

- [x] **6.5 — Evaluation Tools + Confirm flow**
  - `evaluate_listing` Tool (calls the single-shot call from 2.2)
  - `propose_criteria` Tool (pending, no write)
  - `update_context` Tool (pending, no write)
  - Confirm/reject endpoints: `POST .../messages/{mid}/tool-calls/{idx}/confirm`
  - Confirm-Cards in the frontend (Uebernehmen/Verwerfen — means: accept/reject)
  - **Model:** Claude 3.5 Sonnet · GPT-4o (the pending-confirmation pattern must work reliably. The model must not "forget" that it is waiting for confirmation)
  - **Test:** URL in the chat → Agent proposes Criteria → Confirm-Card appears → click Uebernehmen → Criteria are in the DB (visible in 5.1) → evaluate_listing runs

- [x] **6.6 — Remaining Tools**
  - `geocode` Tool (calls 3.4)
  - `list_profile_listings` Tool
  - `compute_ranking` Tool (Score sorting, no LLM. The Agent writes the short summary in the chat. It is persisted only through the button from 5.2)
  - `read_pdf` Tool (PDF upload as a Claude document block; delete the PDF from disk after the ingestion turn)
  - **Model:** Claude 3.5 Sonnet · GPT-4o (PDF input is specific to Claude. OpenCode needs PDF→text extraction instead of a native document block)
  - **Test:** "Vergleiche mit Kappmuren 2" → Agent uses `list_profile_listings`
  - **Test:** "Rangliste aktualisieren" → Agent uses `compute_ranking` and writes the short summary in the chat

---

## Review fixes (intermediate step before Layer 7)

These are bugs and UX gaps found in a manual review of the running app (`docs/review.md`). None of them was planned in a later layer (checked against the whole plan). Everything here is unplanned rework, not pulled-forward future work. It has its own numbering (`RF.x`), so Layer 7/8 do not need to be renumbered.

- [x] **RF.1 — Chat reliability: Confirm-Card status + continuing after the budget limit**
  - The Confirm-Card status (`confirmed`/`rejected`) is persisted in the `chat_message.tool_calls` JSONB. Before, it was only local frontend state. It now survives a tab switch or reload. Confirm and reject are idempotent (409 on a repeated action). See ADR 0017.
  - The `budgetExhausted` branch in `loop.go` persists only text (roundtrip text + limit note). It no longer persists `tool_use` blocks that were not dispatched. This avoids an invalid replay history, which made "weitermachen" (means: "continue") have no effect before.
  - Spec: [[../docs/spec/01_mvp/01-09-chat-reliability-fixes]]
  - **Test:** Confirm a proposal → switch tab/reload → the card still shows "Übernommen". Set `llm.max_llm_roundtrips` low for the test, hit the limit, reply "weitermachen" → the Agent really continues.

- [x] **RF.2 — Token donut readability**
  - `TokenDonut.vue`: ring size and stroke width adjusted, so you can see the fill level at a glance.
  - Spec: [[../docs/spec/01_mvp/01-01-chat-interface#Layout]]
  - **Test:** Visual check in `npm run dev`.

- [x] **RF.3 — Listing List/Detail View rework**
  - Master-detail split (list on the left, detail on the right) in the "Liste" tab, instead of navigating to a separate page. The `/listings/:id` route stays for deep links (chat card).
  - Title-image thumbnails in the list and the detail view.
  - Optional, persistent per-Criterion Score columns in the list (localStorage per profile).
  - Evaluation table at the top of the detail view. Status is shown prominently in the header, not in the form grid.
  - Explicit edit/save/cancel instead of auto-save per field. This bundles changes into one PATCH call.
  - Source URL as a clickable link in display mode.
  - Spec: [[../docs/spec/01_mvp/01-10-listing-views-overhaul]] (replaces parts of 01-05 § List View and 01-07 § Editing Behaviour)
  - **Test:** See the Verification section of the spec.

- [x] **RF.4 — Listing notes (new feature)**
  - Full CRUD on the already existing `listing_note` table (a migration adds `updated_at`): repository, service, API (`/api/listings/{id}/notes`, `/api/notes/{id}`), frontend cards with "Neue Notiz"/edit/delete.
  - Clearly separate from `seller_notes` (the field about the seller, which stays unchanged).
  - Spec: [[../docs/spec/01_mvp/01-11-listing-notes]]
  - **Test:** curl against the new endpoints, then a manual click-through.

- [x] **RF.5 — Chat feedback fixes**
  - Turn-in-progress indicator: a `useChat.ts` flag. It runs from a successful POST until the first live signal for the turn (`chat.text_delta`/`chat.tool_progress`/`chat.message`/`chat.turn_error`). In the meantime `ChatMessageList.vue` shows an animated placeholder. This also covers silent gaps between Tool calls inside one turn.
  - Unicode escape bug (the proposal text shows `\uXXXX` instead of umlauts): not reproducible live. Issue #6 added a defensive decode in `ChatConfirmCard.vue`.
  - Spec: [[../docs/spec/01_mvp/01-12-chat-feedback-fixes]]
  - **Test:** Send a message → the indicator appears at once and disappears at the first text/tool progress. Trigger a multi-roundtrip turn → the indicator appears again at a gap between Tool calls.

- [x] **Issue #23 — Edit LLM configuration via UI**
  - `PATCH /api/config` (partial, validated, write-only API key), `internal/llm.Runtime` swaps both providers live, Settings view at `/settings`.
  - ADR: [[../docs/adr/0022-live-llm-config-runtime]]. Spec: [[../docs/spec/00_infrastructure_setup/00-03-llm-agent-integration#Config]]
  - **Test:** Change the model in the UI → the next chat turn and the next evaluation use it. `GET`/`PATCH /api/config` never contain the key.

---

## Layer 7: Remaining views

- [x] **7.1 — Map View**
  - OpenLayers + OSM map, new "Karte" tab in `ProfileShellView.vue`, lazy loaded (`defineAsyncComponent`)
  - Master-detail split like the List View: map on the left, `ListingDetailPanel` on the right (`ListingSplitPane.vue`, shared with the listing list)
  - One marker per listing with coordinates, color-coded by status (`src/utils/listingStatus.ts`)
  - Marker click: selects the listing for the right column **and** opens a small popup at the marker with title, price, Score, and a 100x100 thumbnail
  - Spec: [[../docs/spec/01_mvp/01-06-map-view]]
  - **Test:** Listing with coordinates (by hand in 5.3 or through the Agent from 6.6) → Karte tab → marker is visible and color-coded → click the marker → popup with thumbnail appears, right column shows the Detail View

- [x] **7.2 — Context Editor**
  - `ContextEditor.vue` — Markdown textarea (edit → save/cancel), preview with marked+DOMPurify in display mode
  - History panel (context_revision list) with restore (confirm dialog, new revision snapshot before the reset)
  - Backend: `GET /api/context/global/history`, `POST .../revisions/{id}/restore` (global + profile). Restore checks that the revision belongs to the target context
  - Global Context through a link on the profile list (`/context`), Profile Context through the profile tab
  - Spec: [[../docs/spec/01_mvp/01-02-context-files]]
  - **Test:** Open a context → change the text → save → history shows a revision → restore sets the old text back (verified with curl, including 400 for a revision from the wrong context) → the Agent reads the context fresh each turn (agent_service.go, unchanged since Layer 6.3)

- [x] **7.3 — Image gallery in the Detail View**
  - `ListingGallery.vue` — thumbnail grid, category badges (Außenansicht/Innenaufnahme/Grundriss/Nebengebäude/Sonstiges — means: exterior/interior/floor plan/outbuilding/other)
  - Star icon to change the title image (PATCH `title_image_id`, already ADR-0012 compliant, no new endpoint needed)
  - Delete with confirmation (`useDialog`, DELETE `/api/listings/{id}/images/{imageId}`, endpoint already exists)
  - Spec: [[../docs/adr/0012-image-storage-filesystem-media]]
  - **Test:** Open the Detail View → gallery shows all images with a category badge → click the star on another image → title image and header thumbnail change (verified live) → upload a test image (curl) → delete icon → confirm dialog → image is removed, title image unchanged (verified live)

---

## Layer 8: Polish

- [x] **8.1 — Rejection async analysis**
  - Status set to `rejected` → async LLM call analyzes the comment (`RejectionAnalysisService`, single-shot like `EvaluationService`/`RankingService`, no tool use)
  - `propose_criteria` / `update_context` proposals land as a synthetic `chat_message` (role=assistant, `tool_calls` in the same format as a real Agent turn) in the active session. The existing Confirm-Card UI/endpoints are reused unchanged
  - Toast "Rejection verarbeitet — Vorschlaege im Chat verfuegbar." (means: rejection processed, proposals are available in the chat) (`useRejectionAnalysisToast.ts`, SSE event `listing.rejection_analyzed`)
  - **Model:** Claude (ADR 0006 — the only MVP provider; single-shot analysis, no tool use)
  - Spec: [[../docs/spec/01_mvp/01-04-criteria-and-scoring#Rejection Feedback, Reimagined]]
  - **Test:** Set a listing to `rejected` with a comment by PATCH (curl + UI) → `update_context` proposal is recognized correctly (broker-avoid rule) → Confirm-Card is visible in the chat (with the Listing Card) → Übernehmen works unchanged → toast confirmed in the browser. golangci-lint/go test clean (including the new `parseRejectionAnalysis` tests).

- [x] **8.2 — user_confirmed_fields in the Agent context**
  - Listing context block before every turn (`BuildListingContextBlock`, `context_builder.go`). The "current listing" comes from the most recent `listing_id` in the session history (`currentListingID`, `agent_service.go`)
  - `save_listing` skips locked fields (already there since ADR 0016)
  - `force_overwrite` after user confirmation (already there since ADR 0016)
  - **Model:** Claude (ADR 0006 — the only MVP provider)
  - Spec: [[../docs/adr/0016-user-confirmed-fields-agent-context]]
  - **Test:** Live against the real Claude API (new chat session because the old session was broken, see the bug list): Price set by hand to 315.000 SEK → on the direct question "welche Felder gesperrt?" (means: which fields are locked?), the Agent correctly answers unprompted with the exact value (no Tool call, the block demonstrably arrived in the system prompt) → Region locked by hand, vague request to change it → Agent refuses and asks back ("nicht überschrieben" — means: not overwritten) → DB confirms it is unchanged → "Ja, überschreib." → Agent updates with `force_overwrite`, the field is correctly removed from `user_confirmed_fields`, DB confirms the change. golangci-lint/go vet/go test clean (including the new `context_builder_test.go`).

- [x] **8.3 — "Neue Session" with warning + disable**
  - Button is disabled while a turn runs (`turnInProgress` in `useChat.ts`, set between a successful POST and `chat.message`/`chat.turn_error`)
  - Dialog warns about open Confirm-Cards (`hasPendingConfirmCards`, shared helper in `src/utils/chat.ts`)
  - Old sessions (read-only, `ended_at` set) can be viewed in a dropdown (already there since Layer 6.1)
  - Bug found during live verification: the `status` in `ChatConfirmCard.vue` was purely local state. Uebernehmen/Verwerfen never updated `messages` in `useChat.ts`. So `hasPendingConfirmCards` stayed stale after a card was resolved. Fix: `ChatConfirmCard.vue` emits `resolved`, `ChatMessageList.vue` passes it on as `callResolved` with the message ID + call index, and `useChat.ts` gets `markToolCallResolved` to patch `messages` in place.
  - **Test:** `turnInProgress` confirmed with timestamps via a MutationObserver (disabled:true at turn start, disabled:false 8.04s later at turn end, not affected by its own Tool roundtrip latency). Confirm-Card ("Testkriterium 8.3b") rejected in the browser → clicked "Neue Session" right away → the dialog correctly shows the simple text with no pending warning (before: the warning wrongly stayed). oxlint/oxfmt/vue-tsc/`npm run build` clean, Docker rebuild.

- [x] **8.4 — Stop button + handling of a missing reply**
  - Backend: `AgentService` holds one `context.CancelFunc` per session (`turns map[uuid.UUID]context.CancelFunc`). It is registered synchronously before `SendMessage` returns (no race with an immediate Stop click). `StopTurn` checks that the session belongs to the profile and calls `cancel()`. New endpoint `POST .../chat/sessions/{sessionId}/stop`.
  - `agent.Loop.Run` checks `ctx.Err()` before every roundtrip. On cancel (before or during streaming, detected with `errors.Is(streamErr, context.Canceled)`), `stopped()` persists a text-only final roundtrip ("[Abgebrochen durch Nutzer.]" — means: stopped by user). It never includes `tool_use` blocks for calls that were not dispatched (same principle as the budget limit). This avoids a dangling `tool_use` on the next turn replay.
  - `dispatchAll` uses `context.WithoutCancel(ctx)` for the actual Tool call. A running Tool call (for example `fetch_url`, `save_listing`) always runs to the end. Only the next LLM request is no longer made.
  - Two bugs found and fixed during live verification: (1) `persistRoundtrip`/`UpdateTokenCount` used the already cancelled `ctx` for the DB write. So the final roundtrip was never persisted and `chat.message` never fired (the frontend stayed stuck in the `turnInProgress` state). Fixed with a `persistCtx := context.WithoutCancel(ctx)`, separate from `ctx`, for all write operations. (2) A cancelled turn with no measured usage (`tokenCount == 0`, because no `ChatEventDone` ever arrived) wrongly overwrote `token_count_estimate` with 0. Fixed with a `tokenCount > 0` guard before the `UpdateTokenCount` call.
  - Frontend: `stop()` in `useChat.ts` calls the new endpoint (a 404 from a race with the natural end of the turn is swallowed). `missingReply` detects a last user message with no follow-up message at all (only possible after a server restart, because a Stop always persists its own final message). `retryLastMessage` sends the same content again (no history editing). `ChatView.vue` shows a "Stop" button next to "Neue Session" while `turnInProgress`. `ChatMessageList.vue` shows "Antwort fehlt — erneut senden?" (means: reply missing, send again?) under the last message when `missingReply`.
  - **Test:** `go test ./internal/agent/...` covers three cancel paths (between roundtrips, in the middle of the stream, Tool call runs to the end despite cancel — `dispatchCtx.Err()` stays `nil` although the outer `ctx` is already cancelled). Live against the real Claude API: clicked the Stop button during a running turn → button disappears, "Neue Session" is active again, the "[Abgebrochen durch Nutzer.]" bubble appears at once in the chat (curl+browser). Compared `token_count_estimate` before/after a cancelled turn (25546 → 25546, not 0) — regression from bug (2) confirmed fixed. Simulated a server restart in the middle of a running turn (`docker compose restart backend` right after the POST) → last message stays `role=user` with no follow-up message → "Antwort fehlt — erneut senden?" appears in the browser → a click resends successfully and the reply comes through completely. golangci-lint/go vet/go test clean.

---

## Dependencies

```
0.1 ──→ 0.2 ──→ 1.1 ──→ 1.2
                    │
                    ├──→ 1.3
                    │
                    ├──→ 2.1 ──→ 2.2 ──→ 2.3
                    │         │
                    │         └──→ 3.1 ──→ 3.2
                    ├──→ 3.3
                    ├──→ 3.4
                    │
                    └──→ 4.1 ──→ 4.2 ──→ 5.1 ──→ 5.2 ──→ 5.3
                                                  │
                                                  └──→ 6.1 ──→ 6.2 ──→ 6.3 ──→ 6.4 ──→ 6.5 ──→ 6.6
                                                                                            │
                                                                                            └──→ RF.1 ──→ RF.2 ──→ RF.3 ──→ RF.4 ──→ RF.5
                                                                                                                                       │
                                                                                                                                       ├──→ 7.1 ──→ 7.2 ──→ 7.3
                                                                                                                                       │
                                                                                                                                       └──→ 8.x (parallel)
```

Layer 7 can run in parallel with Layer 6. The views use the REST APIs from Layers 1-3 and the UI patterns from Layer 5. You can pull 7.2 (Context Editor) forward if needed. Then you can steer the Agent behavior from 6.3 on in the UI instead of with curl.
