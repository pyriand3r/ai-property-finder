---
type: reference
title: "Architecture Decision Records"
project: property-finder-obs-style
created: 2026-07-28
tags:
  - property-finder-obs-style
  - adr
  - index
---

This is a numbered, append-only log of architecture decisions for this project's Chat/Context/Agent redesign. It has its own numbering, separate from the ADR sequence of the predecessor project. Several ADRs here explicitly replace or carry forward specific ADRs of the predecessor project. Each row notes this.

## Index

| ADR | Title | Resolves / affects | Relation to the predecessor project |
|-----|-------|---------------------|--------------------------------------|
| [[0001-chat-first-with-form-fallback]] | Chat is primary; a plain form is a mandatory fallback | [[../../spec/01_mvp/01-01-chat-interface]], [[../../spec/01_mvp/01-07-manual-review-and-edit]] | New |
| [[0002-context-files-freeform-markdown]] | Context Files are freeform Markdown, not structured settings | [[../../spec/01_mvp/01-02-context-files]] | New |
| [[0003-portal-rules-as-context-text]] | Portal-specific handling lives as text in Context, not scraper code | [[../../spec/01_mvp/01-03-listing-ingestion-via-chat]] | Supersedes its anti-bot strategy and HTML reduction decisions |
| [[0004-unified-confirm-before-apply]] | One unified confirm-before-apply mechanism replaces the Proposal entity | [[../../spec/01_mvp/01-02-context-files#Confirmation]], [[../../spec/01_mvp/01-04-criteria-and-scoring]] | Carries forward the principle of its confirm-before-apply decision, replaces its mechanism |
| [[0005-pdf-native-document-input]] | PDF Sources via native Claude document input, no local extraction | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#PDF Handling]] | New (replaces the manual pymupdf step from the reference Obsidian workflow) |
| [[0006-single-provider-for-agent-mvp]] | Claude API is the sole provider for MVP; LM Studio and OpenCode deferred | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Provider Abstraction]] | Narrows the day-1 Claude+LM-Studio requirement of the predecessor project |
| [[0007-agent-loop-provider-independent]] | Agent loop is built provider-independently, not tied to Anthropic's SDK | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/00_infrastructure_setup/00-01-tech-stack]] | Follows from 0006 — the loop stays clean across providers |
| [[0008-agent-turn-fire-and-forget]] | Agent turns run to completion, no recovery on server restart | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/00_infrastructure_setup/00-01-tech-stack]] | Simplifies over the task-persistence model of the predecessor project — recovery not needed for chat-driven turns |
| [[0009-proposal-pending-in-tool-calls]] | Pending proposals live in chat_message.tool_calls JSONB, confirmed via REST | [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/01_mvp/01-02-context-files]] | Implements ADR 0004's confirm-before-apply with no new tables |
| [[0010-tool-result-jsonb-for-replay]] | Tool results stored as JSONB in chat_message, provider-mapped on replay | [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] | Enables lossless conversation history replay across providers |
| [[0011-nominatim-geocoding-tool]] | Geocoding via Nominatim (OSM), added as geocode Tool | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/01_mvp/01-06-map-view]] | Resolves addresses to coordinates for map markers |
| [[0012-image-storage-filesystem-media]] | Image storage: filesystem under /data, served via /media, title_image_id on listing | [[../../spec/00_infrastructure_setup/00-01-tech-stack]], [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] | Persistent volume, static file server, thumbnail via FK |
| [[0013-agent-turn-llm-roundtrip-budget]] | Agent-turn budget: max LLM roundtrips (5), not per-tool or token-based | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] | Controls cost; each Provider.Chat() call counts as one roundtrip |
| [[0014-context-window-token-display-manual-reset]] | Context-window management: token display + manual new-session button, no automatic truncation | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/01_mvp/01-01-chat-interface]] | User sees fill level, decides when to start fresh |
| [[0015-manual-bulk-reevaluation]] | Re-evaluation is manual, not automatic; user triggers per-listing or bulk from UI | [[../../spec/01_mvp/01-04-criteria-and-scoring]], [[../../spec/01_mvp/01-05-ranking-and-list-view]], [[../../spec/01_mvp/01-07-manual-review-and-edit]] | No cost surprise, batch intent, test-before-apply workflow |
| [[0016-user-confirmed-fields-agent-context]] | Agent sees confirmed fields in context; save_listing skips unless force_overwrite | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/01_mvp/01-07-manual-review-and-edit]] | Conversational confirmation, no confirm-card for field overwrites |
| [[0017-tool-call-status-in-jsonb]] | Confirm-Card status persisted as a field inside chat_message.tool_calls JSONB | [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/01_mvp/01-02-context-files#Confirmation]], [[../../spec/01_mvp/01-09-chat-reliability-fixes]] | Extends ADR 0009 — closes the reload-resets-to-pending gap its Consequences section left open. Partly replaced by [[0024-criterion-proposal-status]] (new criteria) |
| [[0018-single-password-jwt-cookie-auth]] | Single-password login, HS256 JWT in an httpOnly cookie | [[../../spec/00_infrastructure_setup/00-01-tech-stack]] | New |
| [[0019-mcp-tool-interface]] | MCP Streamable HTTP interface reuses the existing Agent Tool catalog for external clients | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] | New. Partly replaced by [[0024-criterion-proposal-status]] (`propose_criteria` no longer writes active criteria). Extended by [[0026-external-evaluation-via-mcp]] |
| [[0020-image-upload-native-content-block]] | JPG/PNG screenshots via native Claude image input, same pattern as PDF | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#PDF Handling]] | New (extends ADR 0005 to a second upload format) |
| [[0021-agpl-3-license]] | Project is published under AGPL-3.0-only | `LICENSE`, `package.json` | New |
| [[0022-live-llm-config-runtime]] | LLM settings editable via `PATCH /api/config` + Settings UI, applied live through an atomic snapshot holder | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Config]] | New (extends ADR 0006/0007/0013) |
| [[0023-chat-history-repair-and-atomic-persist]] | Replayed history is repaired (synthetic tool_result for dangling tool_use); each roundtrip is persisted in one transaction | [[../../spec/01_mvp/01-09-chat-reliability-fixes]] | New (extends ADR 0008/0010) |
| [[0024-criterion-proposal-status]] | Criteria are `proposed` or `active`; chat and MCP only propose, the user approves; MCP cannot change active criteria; names unique per profile | [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/01_mvp/01-04-criteria-and-scoring]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] | New (partly replaces ADR 0017 and 0019 for criteria) |
| [[0025-hazard-ratings-region-level-static-data]] | Natural-hazard ratings are region-level, 1-5 (higher is worse) and separate from the Score; static datasets embedded in the binary are synced into `hazard_*` tables at boot; read-only API | [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/01_mvp/01-13-hazard-ratings]] | New |
| [[0026-external-evaluation-via-mcp]] | MCP gets `get_evaluation_brief`, `submit_evaluation`, `list_pending_evaluations`; an external client grades, the server validates and computes the Score; internal evaluation unchanged | [[../../spec/01_mvp/01-14-external-evaluation-via-mcp]], [[../../spec/01_mvp/01-04-criteria-and-scoring]] | New (extends ADR 0019) |
| [[0027-hazard-source-catalogue-and-suggestions]] | Hazard sources are a declarative catalogue per country (bundled / candidate); the Agent and MCP only suggest sources (`suggest_hazard_source`), the user approves before ingestion; ingestion stays a release step | [[../../spec/01_mvp/01-15-hazard-source-catalogue]], [[../../spec/01_mvp/01-13-hazard-ratings]] | New (extends ADR 0025) |
| [[0028-portal-search-task-persistence]] | `search_portal` Tool; a portal search is a `task` row (`portal.search`) with a resumable payload, continued after a restart; narrows ADR 0008 for this task type only | [[../../spec/01_mvp/01-16-portal-search]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] | New (narrows ADR 0008) |

## Notes

- ADR 0003 and 0004 best explain why this project exists as a separate spec and not as a patch to the predecessor project. They replace code-shaped solutions (scraper modules, a Proposal table) with context-shaped ones.
- ADR 0006 is deliberately narrow in scope (which providers, for MVP). It does not ask *whether* a provider abstraction is worth having at all. That is settled by reusing the proven pattern of the predecessor project.
- ADR 0007 follows from 0006 at the implementation level. Other providers *will* arrive after MVP. So the agent loop itself must not be tied to one provider's SDK or wire format. (2026-07-29: using OpenCode instead of Claude as the POC provider was considered and rejected. See the status note in [[0006-single-provider-for-agent-mvp]].)
