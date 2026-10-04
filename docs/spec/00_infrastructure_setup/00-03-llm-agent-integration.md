---
type: spec
created: 2026-07-28
title: LLM Agent Integration (Claude API, tool use)
project: property-finder-obs-style
spec: llm-agent-integration
phase: 0
order: 3
status: ready
updated: 2026-07-28
tags:
  - property-finder-obs-style
  - spec
  - ai
  - llm
  - claude
  - agent
  - tools
  - phase-0
---

## Overview

Replaces [[../../../dev-property-finder/spec/00_infrastructure_setup/00-03-llm-provider-integration|dev-property-finder's single-shot provider interface]] as the primary AI integration for ingestion and conversation. The single-shot operations (`ParseSearchProfile`, `EvaluateListing`) are **kept** — they remain the cleanest way to get one structured result. The Agent invokes them as Tools during chat turns. Outside of chat (manual field edit, Criteria change), the same service functions are called directly by the application.

The agent loop (`internal/agent/loop.go`) is provider-independent per [[../../docs/adr/0007-agent-loop-provider-independent]] — it orchestrates the multi-turn conversation through a normalized `Provider` interface. Claude API is the sole MVP implementation; LM Studio and OpenCode are planned swap targets behind the same interface.

---

## Provider Abstraction

Three providers are in scope for the app long-term, all speaking through one Go interface (`internal/agent.Provider`) the same way dev-property-finder abstracted `ParseSearchProfile`/`EvaluateListing`:

| Provider | Role | Status |
|---|---|---|
| Claude API | Agentic tool use, multi-turn loop, native PDF input | Sole MVP provider |
| LM Studio | Local, OpenAI-compatible inference server | Deferred — interface seam kept, not implemented for MVP |
| OpenCode | OpenAI-compatible endpoint, alternative local/self-hosted backend | Deferred — same seam as LM Studio |

Rationale for shipping Claude-only first: [[../../docs/adr/0006-single-provider-for-agent-mvp]]. The seam matters even though only one implementation exists at MVP — it's what keeps `internal/agent/loop.go` and the Tool dispatch code from hard-coding Claude-specific request/response shapes.

---

## Agent Loop

The loop lives in `internal/agent/loop.go` and is **provider-independent** — it orchestrates the multi-turn conversation through the `Provider` interface without knowing which LLM is on the other side. See [[../../docs/adr/0007-agent-loop-provider-independent]] for the rationale.

One Chat turn:

1. User message (+ optional file) is appended to `chat_message`
2. System prompt is assembled: app-fixed instructions + Global Context (`content_md`) + Profile Context (`content_md`) — in that order, more specific wins on conflict (see [[../01_mvp/01-02-context-files]])
3. Full session history from `chat_message` rows is converted to provider-specific messages via `Provider.FormatMessages()` — `tool_result` JSONB becomes Claude content blocks or OpenAI `tool` role messages depending on provider ([[../../docs/adr/0010-tool-result-jsonb-for-replay]])
4. History + new message + system prompt is sent to the LLM via `Provider.Chat()` with the Tool catalogue, in streaming mode
5. The LLM replies with text deltas and/or tool-call requests over the `ChatEvent` channel; each tool-call is dispatched against the local tool registry, its result persisted as `chat_message` (`role = 'tool'`) with both `content` (human summary for frontend) and `tool_result` (raw JSON for replay), and the loop repeats step 3 until the LLM replies with no further tool calls
6. The final assistant text is streamed to the frontend; each intermediate tool call is also streamed as a progress event so the user sees "Lese Seite…", "Extrahiere Bilder…" live, not just a final answer

Bounded by two limits to prevent runaway loops and control cost:

- **`llm.max_tool_calls_per_turn`** (default 15): maximum number of tool dispatches. Hitting this limit ends the turn with an Agent message explaining what it got through and asking how to proceed.
- **`llm.max_llm_roundtrips`** (default 5): maximum number of LLM API calls (`Provider.Chat()`) per turn. Counts paid roundtrips, not tool dispatches. Hitting this limit returns a tool error to the Agent: „Sage dem User was du geschafft hast und frag ob du weitermachen sollst.“ ([[../../docs/adr/0013-agent-turn-llm-roundtrip-budget]])

### Turn Lifecycle

The turn runs fire-and-forget per [[../../docs/adr/0008-agent-turn-fire-and-forget]]:

1. The user message is persisted to `chat_message` (`role = 'user'`) **before** the loop starts
2. The loop runs in a goroutine with `context.Background()`, detached from the HTTP request lifecycle
3. The SSE stream delivers live progress to the frontend; if the browser disconnects, the loop continues
4. When the loop ends, all Agent messages and tool results are persisted to `chat_message` rows
5. On server restart: in-flight turns die with the process; the frontend detects a user message with no follow-up Agent message and shows an "Antwort fehlt — erneut senden?" button

The `Provider` interface normalizes three differences between LLM backends: tool-definition formatting (`FormatTools`), streaming event structure (`ChatEvent` types), document-input capability flags, and message formatting for history replay (`FormatMessages` — converts DB-neutral `chat_message` rows into provider-specific LLM message structures, including `tool_result` → Claude content blocks or OpenAI `tool` role messages per [[../../docs/adr/0010-tool-result-jsonb-for-replay]]). The Claude implementation (`internal/agent/provider/claude/`) translates Anthropic's native SSE event stream into the normalized `ChatEvent` channel. Future LM Studio / OpenCode implementations will map OpenAI-compatible streaming tool-use events into the same channel.

---

## Tool Catalogue

| Tool | Purpose | Backing implementation |
|---|---|---|
| `fetch_url` | Load a listing page via HTTP GET, extract readable text + all discovered image URLs + all outbound links (for the Agent to decide which secondary link to follow, per Portal Rule) | `internal/fetch` (net/http + readability) |
| `read_pdf` | Read an uploaded PDF as a native Claude document content block — text and embedded images both understood by the model directly | `internal/pdf` |
| `download_image` | Fetch an image URL and store it locally under `/data/images/listings/{id}/{uuid}.ext`. Tags image with a category. Sets `listing.title_image_id` if the listing has no title image yet. Returns `{"image_id", "path"}`. | `internal/fetch` |
| `save_listing` | Create or update a `listing` row with structured fields (title, price, size, address, coordinates, etc.) plus `document_md`. Fields in `user_confirmed_fields` are skipped unless listed in `force_overwrite`. Returns `skipped_fields` when fields were protected ([[../../docs/adr/0016-user-confirmed-fields-agent-context]]). | `internal/db/repository.ListingRepository` |
| `evaluate_listing` | Runs the same Grade/Score computation as [[../../../dev-property-finder/spec/01_mvp/01-12-rate-listing-criteria|dev-property-finder's Evaluation]] — Agent calls this explicitly once a Listing's fields are settled, rather than it firing implicitly on every save | `internal/service` (ported) |
| `propose_criteria` | Propose new or changed Criteria for the current profile — does **not** write to `criterion`; returns `{"status": "pending_confirmation", "summary": "..."}`. The full proposal lives in the tool-call arguments stored in `chat_message.tool_calls`. Confirmed via REST endpoint ([[../../docs/adr/0009-proposal-pending-in-tool-calls]]). | `internal/service` (validates, does not write) |
| `update_context` | Propose an addition/change to Global or Profile Context — does **not** write to `context.content_md`; same pending-confirmation pattern as `propose_criteria`. | `internal/service` (validates, does not write) |
| `list_profile_listings` | Fetch a summary (title, price, score, status, source_url) of existing Listings in the profile, for comparison questions ("vergleiche mit Kappmuren 2") and deduplication | `internal/db/repository.ListingRepository` |
| `compute_ranking` | Fetch the ordered ranking (listings sorted by score DESC) for the current profile. Returns `[{rank, listing_id, title, price, score}]`. Deterministic — no internal LLM call. The Agent writes the Kurzfazit text in its chat reply. | `internal/db/repository.ListingRepository` |
| `geocode` | Resolve an address string to lat/lon coordinates via Nominatim (OSM, no API key). Returns coordinates + display name, or error if not found. | `internal/geocode` |

Each Tool has a JSON schema for its parameters, following the same structured-output discipline as [[../../../dev-property-finder/docs/adr/0006-llm-response-robustness|ADR 0006]]: Claude's native tool-use mode enforces the schema; malformed tool-call arguments are surfaced back to the model as a tool-error result with a corrective note, not retried as a whole new request.

---

## Context Assembly Detail

The system prompt is assembled by `context_builder.go` in this order:

```
[fixed system preamble — see 00-04-system-prompt-preamble]
[Global Context content_md]
[Profile Context content_md]
[listing context: user_confirmed_fields for the current listing, if any — see ADR 0016]
[recent chat_message history for this session]
```

The fixed preamble is a Go string constant in `internal/agent/context_builder.go` — not stored in `context`, not user-editable. Spec: [[00-04-system-prompt-preamble]].

---

## PDF Handling

When the user uploads a PDF (fallback for a blocked/anti-bot page, matching the current Obsidian
workflow's `pymupdf` step), it is sent to Claude as a native `document` content block in the same turn.
No local OCR or image-extraction step is needed for text — Claude reads the PDF directly, including
embedded images, for both the write-up and for photo identification. `download_image` is only invoked
afterwards if the user wants a specific photo copied out and stored against the Listing.

When the user is on a mobile device with no PDF-export option (only able to screenshot a listing), they
can instead upload a JPG or PNG. It is sent to Claude the same way, in the same turn, as a native
`image` content block (ADR 0020) — same downstream flow as the PDF case (`download_image` only
afterwards, only if a specific photo needs to be persisted against the Listing).

After the ingestion turn completes, the PDF or image file is deleted from disk. The extracted data
lives in `listing` + `chat_message` rows. Neither is resent in subsequent turns — this prevents silent
token consumption across sessions ([[../../docs/adr/0014-context-window-token-display-manual-reset]]).

Upload limits: PDF 32 MB (matches Claude's document size limit), JPG/PNG 3.75 MB raw bytes (keeps the
base64-encoded payload within Claude's 5 MB image size limit — base64 inflates by 4/3). Enforced at the
respective multipart form handler (`POST /api/upload/pdf`, `POST /api/upload/image`). Larger files are
rejected before any API call is made.

---

## Error Handling

- `fetch_url` failure (network error, anti-bot block, timeout) is returned to the Agent as a tool error, not a task failure — the Agent decides whether to retry, ask the user for a PDF instead, or give up and say so in the chat. This differs from dev-property-finder's `task.failed` model: there's a human in the loop by construction, so silent-to-error handling isn't needed the same way
- `save_listing` / `evaluate_listing` validation errors (e.g. malformed coordinates) are returned to the Agent as tool errors with the specific field and reason, so it can ask the user or self-correct within the same turn
- If Claude's response itself is unparseable at the transport level (rare, with native tool-use mode), the same retry-with-corrective-instruction pattern as [[../../../dev-property-finder/docs/adr/0006-llm-response-robustness|ADR 0006]] applies, capped at `llm.max_retries`

---

## Config

Carried over from dev-property-finder's `app_config`: `llm.provider` (fixed to `claude` for MVP; `lmstudio` / `opencode` reserved values for when those implementations land), `llm.api_key`, `llm.base_url` (unused for `claude`, required for `lmstudio`/`opencode`), `llm.model`, `llm.timeout_seconds`, `llm.max_retries`. New: `llm.max_tool_calls_per_turn` (default 15), `llm.max_llm_roundtrips` (default 5, [[../../docs/adr/0013-agent-turn-llm-roundtrip-budget]]), `llm.reasoning_language` (default `de`, governs both Grade reasoning and chat replies).

---

## Open Questions / TBD

- Whether LM Studio's and OpenCode's tool-use support (format, reliability, streaming) is close enough to Claude's that `internal/agent.Provider` can stay a thin interface, or whether the Agent loop itself needs provider-specific branches once one of them is actually implemented — unverified until [[../../docs/adr/0006-single-provider-for-agent-mvp]] is revisited

---

## MCP Interface

`POST /mcp` (`internal/mcpserver`, docs/adr/0019) exposes this Tool Catalog over the MCP Streamable HTTP
transport for external clients (Claude Code, Claude Desktop via the `mcp-remote` proxy, opencode, omp),
authenticated via `Authorization: Bearer <APP_PASSWORD>` instead of the browser's session cookie. Ten
tools: the nine above, each dispatched through the same `agent.Registry.Dispatch` the chat Agent uses, plus
`list_profiles` (bootstrap discovery of `profile_id`, not available any other way outside a chat session).
`propose_criteria`/`update_context` write immediately over MCP (no chat confirm-card exists outside a real
`chat_message`) — see ADR 0019 for the full reasoning.

