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

This spec replaces the predecessor project's single-shot provider interface as the main AI integration for ingestion and conversation. The single-shot operations (`ParseSearchProfile`, `EvaluateListing`) are **kept**. They are still the cleanest way to get one structured result. The Agent calls them as Tools during chat turns. Outside of chat (manual field edit, Criteria change), the application calls the same service functions directly.

The agent loop (`internal/agent/loop.go`) does not depend on one provider, per [[../../docs/adr/0007-agent-loop-provider-independent]]. It runs the multi-turn conversation through a normalized `Provider` interface. The Claude API is the only MVP implementation. LM Studio and OpenCode are planned replacements behind the same interface.

---

## Provider Abstraction

Three providers are in scope for the app long-term. All of them talk through one Go interface (`internal/agent.Provider`). The predecessor project abstracted `ParseSearchProfile`/`EvaluateListing` the same way.

| Provider | Role | Status |
|---|---|---|
| Claude API | Agentic tool use, multi-turn loop, native PDF input | Only MVP provider |
| LM Studio | Local, OpenAI-compatible inference server | Deferred. The interface seam is kept, but it is not implemented for MVP |
| OpenCode | OpenAI-compatible endpoint, alternative local/self-hosted backend | Deferred. Same seam as LM Studio |

The reason for shipping Claude-only first: [[../../docs/adr/0006-single-provider-for-agent-mvp]]. The seam matters even though only one implementation exists at MVP. It stops `internal/agent/loop.go` and the Tool dispatch code from hard-coding Claude-specific request and response shapes.

---

## Agent Loop

The loop lives in `internal/agent/loop.go`. It does **not depend on one provider**. It runs the multi-turn conversation through the `Provider` interface and does not know which LLM is on the other side. See [[../../docs/adr/0007-agent-loop-provider-independent]] for the reason.

One Chat turn:

1. The user message (plus an optional file) is appended to `chat_message`.
2. The system prompt is assembled: app-fixed instructions, then Global Context (`content_md`), then Profile Context (`content_md`). The more specific context wins on conflict (see [[../01_mvp/01-02-context-files]]).
3. The full session history from `chat_message` rows is converted to provider-specific messages with `Provider.FormatMessages()`. The `tool_result` JSONB becomes Claude content blocks or OpenAI `tool` role messages, depending on the provider ([[../../docs/adr/0010-tool-result-jsonb-for-replay]]).
4. History, new message and system prompt are sent to the LLM with `Provider.Chat()`, together with the Tool catalogue, in streaming mode.
5. The LLM replies with text deltas and/or tool-call requests over the `ChatEvent` channel. Each tool call is dispatched against the local tool registry. Its result is saved as a `chat_message` (`role = 'tool'`) with two parts: `content` (a human summary for the frontend) and `tool_result` (raw JSON for replay). The loop then repeats step 3 until the LLM replies with no more tool calls.
6. The final assistant text is streamed to the frontend. Each intermediate tool call is also streamed as a progress event. This way the user sees "Lese Seite…" and "Extrahiere Bilder…" live, not just a final answer. (These German strings mean "Reading page…" and "Extracting images…".)

Two limits stop runaway loops and control cost:

- **`llm.max_tool_calls_per_turn`** (default 15): the maximum number of tool dispatches. When the Agent hits this limit, the turn ends with an Agent message. The message explains what the Agent got through and asks how to proceed.
- **`llm.max_llm_roundtrips`** (default 5): the maximum number of LLM API calls (`Provider.Chat()`) per turn. It counts paid roundtrips, not tool dispatches. When the Agent hits this limit, it gets a tool error: „Sage dem User was du geschafft hast und frag ob du weitermachen sollst.“ (means: "Tell the user what you got done and ask if you should continue.") See [[../../docs/adr/0013-agent-turn-llm-roundtrip-budget]].

### Turn Lifecycle

The turn runs fire-and-forget, per [[../../docs/adr/0008-agent-turn-fire-and-forget]]:

1. The user message is saved to `chat_message` (`role = 'user'`) **before** the loop starts.
2. The loop runs in a goroutine with `context.Background()`. It is detached from the HTTP request lifecycle.
3. The SSE stream sends live progress to the frontend. If the browser disconnects, the loop keeps running.
4. When the loop ends, all Agent messages and tool results are saved as `chat_message` rows.
5. On server restart, in-flight turns die with the process. The frontend finds a user message with no follow-up Agent message. It then shows an "Antwort fehlt — erneut senden?" button (means: "Answer missing — send again?").

The `Provider` interface normalizes four differences between LLM backends:

- tool-definition formatting (`FormatTools`)
- streaming event structure (`ChatEvent` types)
- document-input capability flags
- message formatting for history replay (`FormatMessages`). It converts DB-neutral `chat_message` rows into provider-specific LLM message structures. This includes `tool_result` → Claude content blocks or OpenAI `tool` role messages, per [[../../docs/adr/0010-tool-result-jsonb-for-replay]].

The Claude implementation (`internal/agent/provider/claude/`) translates Anthropic's native SSE event stream into the normalized `ChatEvent` channel. Future LM Studio / OpenCode implementations will map OpenAI-compatible streaming tool-use events into the same channel.

---

## Tool Catalogue

| Tool | Purpose | Backing implementation |
|---|---|---|
| `fetch_url` | Load a listing page with HTTP GET. Extract readable text, all discovered image URLs and all outbound links. The Agent uses the links to decide which secondary link to follow, per Portal Rule. | `internal/fetch` (net/http + readability) |
| `read_pdf` | Read an uploaded PDF as a native Claude document content block. The model understands both the text and the embedded images directly. | `internal/pdf` |
| `download_image` | Fetch an image URL and store it locally under `/data/images/listings/{id}/{uuid}.ext`. Tag the image with a category. Set `listing.title_image_id` if the listing has no title image yet. Returns `{"image_id", "path"}`. | `internal/fetch` |
| `save_listing` | Create or update a `listing` row with structured fields (title, price, size, address, coordinates, etc.) plus `document_md`. Fields in `user_confirmed_fields` are skipped unless listed in `force_overwrite`. Returns `skipped_fields` when fields were protected ([[../../docs/adr/0016-user-confirmed-fields-agent-context]]). | `internal/db/repository.ListingRepository` |
| `evaluate_listing` | Run the same Grade/Score computation as the predecessor project's Evaluation. The Agent calls this explicitly once a Listing's fields are settled. It does not fire implicitly on every save. | `internal/service` (ported) |
| `propose_criteria` | Propose new or changed Criteria for the current profile. `action=add` stores the criterion with `status = 'proposed'` at once (ignored by evaluation until approved) and returns `{"status": "pending_confirmation", "criterion_id": "...", "summary": "..."}`. The Confirm-Card approves that row, or deletes it on reject ([[../../docs/adr/0024-criterion-proposal-status]]). `action=edit` does **not** write: the full proposal lives in the tool-call arguments stored in `chat_message.tool_calls` and is applied on confirm ([[../../docs/adr/0009-proposal-pending-in-tool-calls]]). | `internal/service` (`CriterionService.Propose`) |
| `update_context` | Propose an addition or change to Global or Profile Context. It does **not** write to `context.content_md`. It uses the same pending-confirmation pattern as `propose_criteria`. | `internal/service` (validates, does not write) |
| `list_profile_listings` | Fetch a summary (title, price, score, status, source_url) of existing Listings in the profile. Used for comparison questions ("vergleiche mit Kappmuren 2", means: "compare with Kappmuren 2") and for deduplication. | `internal/db/repository.ListingRepository` |
| `compute_ranking` | Fetch the ordered ranking (listings sorted by score DESC) for the current profile. Returns `[{rank, listing_id, title, price, score}]`. It is deterministic and makes no internal LLM call. The Agent writes the Kurzfazit text (the short summary) in its chat reply. | `internal/db/repository.ListingRepository` |
| `geocode` | Resolve an address string to lat/lon coordinates through Nominatim (OSM, no API key). Returns coordinates and a display name, or an error if nothing is found. | `internal/geocode` |

Each Tool has a JSON schema for its parameters. This follows the same structured-output rules as the predecessor project's LLM-response robustness patterns. Claude's native tool-use mode enforces the schema. If the arguments of a tool call are malformed, the app sends them back to the model as a tool-error result with a corrective note. It does not retry as a whole new request.

---

## Context Assembly Detail

`context_builder.go` assembles the system prompt in this order:

```
[fixed system preamble — see 00-04-system-prompt-preamble]
[Global Context content_md]
[Profile Context content_md]
[listing context: user_confirmed_fields for the current listing, if any — see ADR 0016]
[recent chat_message history for this session]
```

The fixed preamble is a Go string constant in `internal/agent/context_builder.go`. It is not stored in `context`, and the user cannot edit it. Spec: [[00-04-system-prompt-preamble]].

---

## PDF Handling

The user can upload a PDF as a fallback for a blocked or anti-bot page. This matches the `pymupdf` step in the reference Obsidian workflow. The app sends the PDF to Claude as a native `document` content block in the same turn. No local OCR or image-extraction step is needed for text. Claude reads the PDF directly, including embedded images. This works for the write-up and for photo identification. The app calls `download_image` only afterwards, and only if the user wants a specific photo copied out and stored on the Listing.

On a mobile device, the user may have no PDF-export option and can only screenshot a listing. In that case the user can upload a JPG or PNG instead. The app sends it to Claude the same way, in the same turn, as a native `image` content block (ADR 0020). The flow after that is the same as for a PDF: `download_image` runs only afterwards, and only if a specific photo must be saved on the Listing.

After the ingestion turn completes, the app deletes the PDF or image file from disk. The extracted data lives in `listing` and `chat_message` rows. The app does not resend the file in later turns. This stops silent token use across sessions ([[../../docs/adr/0014-context-window-token-display-manual-reset]]).

Upload limits:

- PDF: 32 MB (matches Claude's document size limit).
- JPG/PNG: 3.75 MB raw bytes. This keeps the base64-encoded payload within Claude's 5 MB image size limit, because base64 makes data 4/3 larger.

The respective multipart form handlers enforce these limits (`POST /api/upload/pdf`, `POST /api/upload/image`). The app rejects larger files before it makes any API call.

---

## Error Handling

- A `fetch_url` failure (network error, anti-bot block, timeout) is returned to the Agent as a tool error, not as a task failure. The Agent decides whether to retry, ask the user for a PDF instead, or give up and say so in the chat. This differs from the predecessor project's `task.failed` model. A human is in the loop by design, so errors do not need to be handled silently in the same way.
- `save_listing` / `evaluate_listing` validation errors (for example malformed coordinates) are returned to the Agent as tool errors with the specific field and the reason. The Agent can then ask the user or fix the problem itself in the same turn.
- If Claude's response cannot be parsed at the transport level (rare with native tool-use mode), the same retry-with-corrective-instruction pattern as in the predecessor project applies. It is capped at `llm.max_retries`.

---

## Config

Stored as key-value rows in `app_config` (category `llm`). The DB is the source of truth. `CLAUDE_API_KEY` from the environment is written once, on the very first boot, as `llm.api_key` (`config.SeedFromEnv`) and is never read again.

- `llm.provider`: `claude` is the only implemented provider (default). `lmstudio` / `opencode` are reserved values for when those implementations exist. The selectable providers come from the backend (`llm.Providers()`), not from the frontend.
- `llm.api_key`: write-only over HTTP (see "Editing" below).
- `llm.base_url`: optional for `claude` (proxy/gateway override), required for `lmstudio`/`opencode`.
- `llm.model` (default `claude-sonnet-5`)
- `llm.timeout_seconds` (default 30, allowed 1–600)
- `llm.max_retries` (default 2, allowed 0–10)
- `llm.max_tool_calls_per_turn` (default 15, allowed 1–100)
- `llm.max_llm_roundtrips` (default 5, allowed 1–50, [[../../docs/adr/0013-agent-turn-llm-roundtrip-budget]])
- `llm.reasoning_language` (default `de`). It controls both Grade reasoning and chat replies.

### Editing (docs/adr/0022)

- `GET /api/config` returns `{ "llm": {...}, "providers": [...] }`. `llm` has every setting above except the key, plus `api_key_configured: true/false`. `providers` is a list of `{ id, label, models, requires_base_url }`. `models` are suggestions for the model picker, not an allow-list.
- `PATCH /api/config` takes `{ "llm": { ...any subset of the fields above, plus api_key } }` and returns the same body as `GET`. Invalid values return 400 with a German message (`domain.ValidationError`): unknown provider, empty model, `base_url` that is not an http(s) URL, a number outside its range, empty `reasoning_language`. An invalid request changes nothing.
- The API key is **write-only**. It is never returned by any endpoint and never logged (`config.LLM.String()` redacts it). A `PATCH` without `api_key` keeps the stored key. An empty `api_key` is rejected.
- Changes apply **without a restart**. `internal/llm.Runtime` rebuilds both providers and swaps them atomically after the DB write succeeds. The next evaluation, ranking or rejection analysis, and the next Agent turn, use the new values. An Agent turn that is already running keeps the settings it started with.
- The SPA has a Settings view at `/settings` (German UI, linked from the profile list).
- Edits made directly in SQL are not picked up by the running process. Restart the backend, or save once in the Settings view.

## Open Questions / TBD

- Is the tool-use support of LM Studio and OpenCode (format, reliability, streaming) close enough to Claude's? If yes, `internal/agent.Provider` can stay a thin interface. If not, the Agent loop itself may need provider-specific branches once one of them is implemented. This is unverified until [[../../docs/adr/0006-single-provider-for-agent-mvp]] is revisited.

---

## MCP Interface

`POST /mcp` (`internal/mcpserver`, docs/adr/0019) exposes this Tool Catalog over the MCP Streamable HTTP transport. It is for external clients (Claude Code, Claude Desktop via the `mcp-remote` proxy, opencode, omp). These clients authenticate with `Authorization: Bearer <APP_PASSWORD>` instead of the browser's session cookie. There are ten tools. Nine are the tools above, and each is dispatched through the same `agent.Registry.Dispatch` that the chat Agent uses. The tenth is `list_profiles`. It lets a client discover a `profile_id` at the start, which is not possible any other way outside a chat session. Over MCP, `update_context` writes immediately, because no chat Confirm-Card exists outside a real `chat_message`. See ADR 0019 for the full reasoning. `propose_criteria` only writes criteria with `status = 'proposed'`: `add` creates one, `edit` works only on a criterion that is still proposed, and `edit` on an active criterion is an error. The result contains the criterion's `status`. The user approves in the Criteria tab ([[../../docs/adr/0024-criterion-proposal-status]]).
