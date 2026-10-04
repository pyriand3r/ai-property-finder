---
type: adr
title: "MCP Streamable HTTP interface reusing the existing Agent Tool catalog"
project: property-finder-obs-style
status: accepted
created: 2026-09-27
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]].

## Context

The app's capabilities were reachable two ways: the in-app chat Agent's Tool catalog
(`internal/agent/tools.go`'s `Tools()` — `fetch_url`, `save_listing`, `download_image`,
`evaluate_listing`, `propose_criteria`, `update_context`, `geocode`, `list_profile_listings`,
`compute_ranking`) and the REST API + plain forms. External MCP-capable clients (Claude Code, Claude
Desktop, opencode, omp) had no way to drive this app's capabilities directly.

## Decision

A new `POST /mcp` endpoint speaks the MCP Streamable HTTP transport (`github.com/modelcontextprotocol
/go-sdk`, the official Go SDK — chosen over `mark3labs/mcp-go` the same way `anthropic-sdk-go` is this
repo's only Claude SDK: prefer the vendor/official SDK when both exist). It exposes exactly the existing
Tool catalog, dispatched through the same `agent.Registry.Dispatch` code path the internal chat Agent uses
(`internal/service.NewToolRegistry`, extracted from `AgentService.runTurn` so both share one construction),
plus one new tool, `list_profiles`, needed because none of the nine existing tools has any way to discover
a `profile_id` outside a real chat session.

`propose_criteria` and `update_context` write immediately over MCP instead of staging a chat confirm-card
proposal (ADR 0004/0009) — that mechanism is tied to a real `chat_message` row, which doesn't exist for an
MCP call. An MCP call is already interactively driven by a human operating the external client, the same
trust level as using the plain Criteria/Context forms (ADR 0001) — so `propose_criteria`/`update_context`
call `CriterionService`/`ContextService` directly (the exact same calls the REST endpoints and, once
confirmed, a chat proposal already make), recording context edits as `ChangedByUser`, not `ChangedByAgent`
(there's no `chat_message` to attribute them to).

Auth: a static `Authorization: Bearer <token>` header, checked against `APP_PASSWORD` — the same single
secret ADR 0018 established, no second secret introduced. MCP clients cannot perform `RequireAuth`'s
interactive cookie login, so `RequireBearer` is a sibling `internal/api` middleware guarding only `/mcp`.

## Alternatives considered

- **Expose the full REST CRUD surface (profiles, notes, images, chat, ranking-refresh, etc.) as MCP
  tools** — rejected: the user asked for "all the defined possibilities" — the literal, already-named Tool
  catalog (AGENTS.md's own "Tool" glossary entry lists these exact nine names) — not a second, parallel
  implementation of the whole REST API. `list_profiles` is the one addition genuinely required to make the
  existing catalog usable outside a chat session at all.
- **`mark3labs/mcp-go`** — rejected in favor of the official, Google-maintained
  `github.com/modelcontextprotocol/go-sdk`, matching this repo's one-official-SDK-per-integration pattern.
- **OAuth 2.1 for MCP auth** (would let Claude Desktop's native custom-connector UI, which only accepts
  OAuth fields, connect directly) — rejected: a full authorization server (dynamic client registration,
  PKCE, token issuance/validation, Protected Resource Metadata) is exactly the complexity ADR 0018 already
  rejected for a single-user self-hosted app. Claude Desktop can still reach `/mcp` via the community
  `mcp-remote` stdio proxy (`npx mcp-remote <url> --header "Authorization: Bearer <token>"`), which Claude
  Desktop's local-stdio-server support already handles — a documented workaround, not a missing feature.
- **A separate `MCP_TOKEN` env var** instead of reusing `APP_PASSWORD` — rejected for the same reason ADR
  0018 rejected a separate JWT secret: one password is simpler for a single-user app, and there's exactly
  one legitimate operator.

## Consequences

- No new env var — `/mcp` reuses `APP_PASSWORD`; rotating it invalidates MCP access exactly like it
  invalidates browser sessions (ADR 0018's existing behavior, now covering a second surface).
- Claude Code and opencode connect directly with a static bearer header; Claude Desktop needs the
  `mcp-remote` local-stdio-proxy workaround (its custom-connector UI has no static-bearer-token field, only
  OAuth) — documented, not fixed, per Alternatives above.
- `propose_criteria`/`update_context` over MCP have no undo/confirm step, unlike their chat-driven
  namesakes — an MCP-issued criterion or context edit is immediate and permanent the moment the tool
  returns, same as using the plain form.
- `internal/service.NewToolRegistry` is now the one place that builds a dispatch-ready `agent.Registry`;
  any future Tool catalog change (new tool, new adapter) only needs updating in one place for both the chat
  Agent and MCP to pick it up.
