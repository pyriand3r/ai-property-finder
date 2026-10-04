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

Numbered, append-only log of architecture decisions for this project's Chat/Context/Agent redesign. Own numbering, independent of [[../../../dev-property-finder/docs/adr/README|dev-property-finder's ADR sequence]] — several ADRs here explicitly supersede or carry forward specific dev-property-finder ADRs, noted per row.

## Index

| ADR | Title | Resolves / affects | Relation to dev-property-finder |
|-----|-------|---------------------|----------------------------------|
| [[0001-chat-first-with-form-fallback]] | Chat is primary; a plain form is a mandatory fallback | [[../../spec/01_mvp/01-01-chat-interface]], [[../../spec/01_mvp/01-07-manual-review-and-edit]] | New |
| [[0002-context-files-freeform-markdown]] | Context Files are freeform Markdown, not structured settings | [[../../spec/01_mvp/01-02-context-files]] | New |
| [[0003-portal-rules-as-context-text]] | Portal-specific handling lives as text in Context, not scraper code | [[../../spec/01_mvp/01-03-listing-ingestion-via-chat]] | Supersedes ADR 0007, ADR 0008 |
| [[0004-unified-confirm-before-apply]] | One unified confirm-before-apply mechanism replaces the Proposal entity | [[../../spec/01_mvp/01-02-context-files#Confirmation]], [[../../spec/01_mvp/01-04-criteria-and-scoring]] | Carries forward the principle of ADR 0001, replaces its mechanism |
| [[0005-pdf-native-document-input]] | PDF Sources via native Claude document input, no local extraction | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#PDF Handling]] | New (replaces the manual pymupdf step from the reference Obsidian workflow) |
| [[0006-single-provider-for-agent-mvp]] | Claude API is the sole provider for MVP; LM Studio and OpenCode deferred | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Provider Abstraction]] | Narrows dev-property-finder's day-1 Claude+LM-Studio requirement |
| [[0007-agent-loop-provider-independent]] | Agent loop is built provider-independently, not tied to Anthropic's SDK | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/00_infrastructure_setup/00-01-tech-stack]] | Follows from 0006 — the loop stays clean across providers |
| [[0008-agent-turn-fire-and-forget]] | Agent turns run to completion, no recovery on server restart | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/00_infrastructure_setup/00-01-tech-stack]] | Simplifies over dev-property-finder's task-persistence model (ADR 0004) — recovery not needed for chat-driven turns |
| [[0009-proposal-pending-in-tool-calls]] | Pending proposals live in chat_message.tool_calls JSONB, confirmed via REST | [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/01_mvp/01-02-context-files]] | Implements ADR 0004's confirm-before-apply with no new tables |
| [[0010-tool-result-jsonb-for-replay]] | Tool results stored as JSONB in chat_message, provider-mapped on replay | [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] | Enables lossless conversation history replay across providers |
| [[0011-nominatim-geocoding-tool]] | Geocoding via Nominatim (OSM), added as geocode Tool | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/01_mvp/01-06-map-view]] | Resolves addresses to coordinates for map markers |
| [[0012-image-storage-filesystem-media]] | Image storage: filesystem under /data, served via /media, title_image_id on listing | [[../../spec/00_infrastructure_setup/00-01-tech-stack]], [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] | Persistent volume, static file server, thumbnail via FK |
| [[0013-agent-turn-llm-roundtrip-budget]] | Agent-turn budget: max LLM roundtrips (5), not per-tool or token-based | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] | Controls cost; each Provider.Chat() call counts as one roundtrip |
| [[0014-context-window-token-display-manual-reset]] | Context-window management: token display + manual new-session button, no automatic truncation | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/01_mvp/01-01-chat-interface]] | User sees fill level, decides when to start fresh |
| [[0015-manual-bulk-reevaluation]] | Re-evaluation is manual, not automatic; user triggers per-listing or bulk from UI | [[../../spec/01_mvp/01-04-criteria-and-scoring]], [[../../spec/01_mvp/01-05-ranking-and-list-view]], [[../../spec/01_mvp/01-07-manual-review-and-edit]] | No cost surprise, batch intent, test-before-apply workflow |
| [[0016-user-confirmed-fields-agent-context]] | Agent sees confirmed fields in context; save_listing skips unless force_overwrite | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]], [[../../spec/01_mvp/01-07-manual-review-and-edit]] | Conversational confirmation, no confirm-card for field overwrites |
| [[0017-tool-call-status-in-jsonb]] | Confirm-Card status persisted as a field inside chat_message.tool_calls JSONB | [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/01_mvp/01-02-context-files#Confirmation]], [[../../spec/01_mvp/01-09-chat-reliability-fixes]] | Extends ADR 0009 — closes the reload-resets-to-pending gap its Consequences section left open |
| [[0018-single-password-jwt-cookie-auth]] | Single-password login, HS256 JWT in an httpOnly cookie | [[../../spec/00_infrastructure_setup/00-01-tech-stack]] | New |
| [[0019-mcp-tool-interface]] | MCP Streamable HTTP interface reuses the existing Agent Tool catalog for external clients | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]] | New |
| [[0020-image-upload-native-content-block]] | JPG/PNG screenshots via native Claude image input, same pattern as PDF | [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#PDF Handling]] | New (extends ADR 0005 to a second upload format) |

## Notes

- ADR 0003 and 0004 are the two decisions that most directly explain why this project exists as a separate spec rather than a patch to dev-property-finder — they replace code-shaped solutions (scraper modules, a Proposal table) with context-shaped ones.
- ADR 0006 is deliberately narrow in scope (which providers, for MVP) — it does not revisit *whether* a provider abstraction is worth having at all; that's settled by reusing dev-property-finder's proven pattern.
- ADR 0007 is the implementation-level consequence of 0006: because alternative providers *will* arrive post-MVP, the agent loop itself must not be coupled to any one provider's SDK or wire format. (2026-07-29: OpenCode statt Claude als POC-Provider wurde erwogen und verworfen — siehe Status-Notiz in [[0006-single-provider-for-agent-mvp]].)
