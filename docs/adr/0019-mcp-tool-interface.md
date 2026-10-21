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

Accepted. This affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]].

## Context

The app's features could be reached in two ways. One was the Tool catalog of the in-app chat Agent
(`Tools()` in `internal/agent/tools.go`: `fetch_url`, `save_listing`, `download_image`,
`evaluate_listing`, `propose_criteria`, `update_context`, `geocode`, `list_profile_listings`,
`compute_ranking`). The other was the REST API plus plain forms. External clients that support MCP
(Claude Code, Claude Desktop, opencode, omp) had no way to use this app's features directly.

## Decision

A new `POST /mcp` endpoint speaks the MCP Streamable HTTP transport. It uses
`github.com/modelcontextprotocol/go-sdk`, the official Go SDK. This was chosen over `mark3labs/mcp-go` for
the same reason `anthropic-sdk-go` is this repo's only Claude SDK: when both exist, prefer the
vendor/official SDK. The endpoint exposes exactly the existing Tool catalog. Calls go through the same
`agent.Registry.Dispatch` code path that the internal chat Agent uses. `internal/service.NewToolRegistry`
was extracted from `AgentService.runTurn`, so both share one construction. The endpoint adds one new tool,
`list_profiles`. It is needed because none of the nine existing tools can find a `profile_id` outside a
real chat session.

Over MCP, `propose_criteria` and `update_context` write immediately. They do not stage a chat Confirm-Card
proposal (ADR 0004/0009). That mechanism depends on a real `chat_message` row, and an MCP call has none.
A human already drives every MCP call through the external client. This is the same trust level as using
the plain Criteria/Context forms (ADR 0001). So `propose_criteria` and `update_context` call
`CriterionService` and `ContextService` directly. These are the same calls that the REST endpoints make,
and that a chat proposal makes once confirmed. Context edits are recorded as `ChangedByUser`, not
`ChangedByAgent`, because there is no `chat_message` to credit them to.

Auth: a static `Authorization: Bearer <token>` header, checked against `APP_PASSWORD`. This is the same
single secret that ADR 0018 set up, so no second secret is added. MCP clients cannot do the interactive
cookie login of `RequireAuth`. So `RequireBearer` is a sibling `internal/api` middleware that guards only
`/mcp`.

## Alternatives considered

- **Expose the full REST CRUD surface (profiles, notes, images, chat, ranking-refresh, etc.) as MCP
  tools** — rejected. The user asked for "all the defined possibilities". That means the literal,
  already-named Tool catalog (the "Tool" glossary entry in AGENTS.md lists these exact nine names). It
  does not mean a second, parallel implementation of the whole REST API. `list_profiles` is the one
  addition that is truly needed to make the existing catalog usable outside a chat session at all.
- **`mark3labs/mcp-go`** — rejected in favor of the official, Google-maintained
  `github.com/modelcontextprotocol/go-sdk`. This matches the repo's pattern of one official SDK per
  integration.
- **OAuth 2.1 for MCP auth** — rejected. It would let the native custom-connector UI of Claude Desktop
  connect directly, because that UI only accepts OAuth fields. But a full authorization server needs
  dynamic client registration, PKCE, token issuance and validation, and Protected Resource Metadata.
  ADR 0018 already rejected exactly this complexity for a single-user self-hosted app. Claude Desktop can
  still reach `/mcp` through the community `mcp-remote` stdio proxy
  (`npx mcp-remote <url> --header "Authorization: Bearer <token>"`). Claude Desktop's support for local
  stdio servers already handles it. This is a documented workaround, not a missing feature.
- **A separate `MCP_TOKEN` env var** instead of reusing `APP_PASSWORD` — rejected, for the same reason
  ADR 0018 rejected a separate JWT secret. One password is simpler for a single-user app, and there is
  exactly one legitimate operator.

## Consequences

- There is no new env var. `/mcp` reuses `APP_PASSWORD`. Changing it ends MCP access in the same way it
  ends browser sessions (the existing behavior of ADR 0018, now covering a second surface).
- Claude Code and opencode connect directly with a static bearer header. Claude Desktop needs the
  `mcp-remote` local-stdio-proxy workaround, because its custom-connector UI has no static-bearer-token
  field, only OAuth. This is documented, not fixed, as described in Alternatives above.
- Over MCP, `propose_criteria` and `update_context` have no undo or confirm step, unlike their chat-driven
  namesakes. A criterion or context edit made over MCP is immediate and permanent as soon as the tool
  returns. This is the same as using the plain form.
- `internal/service.NewToolRegistry` is now the one place that builds a dispatch-ready `agent.Registry`.
  Any later change to the Tool catalog (a new tool, a new adapter) needs an update in only one place.
  Both the chat Agent and MCP then pick it up.
