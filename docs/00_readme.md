---
type: project
created: 2026-07-28
title: Property Finder OBS-Style — Project Description
project: property-finder-obs-style
status: active
started: 2026-07-28
due:
goal: Web app spec for a chat-driven real-estate finder that replaces the proven Obsidian workflow
tags:
  - property-finder-obs-style
  - beschreibung
  - konzept
---
## Description

This project follows a predecessor project. The old spec was fully planned (12 ADRs, full MVP). But it modeled the **wrong workflow**: form-based CRUD with hard-coded scraper modules for each portal. The workflow that really works, and that is used in daily practice, runs in Obsidian. An AI in a chat gets a URL (or a PDF). It reads the listing and follows portal-specific links. It extracts images and writes a structured document. It evaluates the listing openly by criteria. It also keeps the ranking and the map up to date.

This app idea moves exactly that workflow into a web app. The chat window is the core, not forms. Each Search Profile has its own persistent, editable context. This is the same as a profile-specific AGENT.md. There is also a global context. This is the same as the vault-wide AGENT.md.

The open A–F grading scheme with weighted criteria from the old spec stays. The model was never the problem. The problem was only how data got into it.

## Core idea

- **Chat instead of forms** as the main input. The user gives the AI instructions in the chat ("evaluate this listing", "follow the agent link", "compare with the others"). There are no multi-step forms.
- **Agentic listing processing**: the AI gets a URL or PDF. It calls Tools (load page, extract images, follow agent link). It writes a structured record and a readable full text (original + translation + evaluation + pros/cons — like today's Obsidian files).
- **Two context levels, like AGENT.md**:
  - Global context: app-wide base rules. Some are pre-filled (image extraction, table format, star rating). The user can extend them.
  - Profile context: each Search Profile has its own instructions (for example "for Swedish properties always check water/sewage and follow the Booli agent link").
- **Transparent grading scheme**: criteria defined by the AI, an A–F grade for each criterion, and a weighted total Score. This works as in the predecessor project. But you can edit criteria directly, not only by re-parsing free text.
- **List, Ranking, map**: like the Obsidian dashboard — a sorted list, an AI-made ranking with a short summary, and an OSM map with coordinates.
- **Full traceability**: every field the AI fills stays editable by hand. Changes trigger a re-evaluation.

## Why the old approach was wrong

| Old (predecessor project) | New (this project) |
|---|---|
| Form CRUD for each field | Chat as the main interface |
| One free text per profile → AI parses criteria once | Ongoing dialog. Criteria appear and grow step by step, and you can edit them directly |
| Hard-coded scraper + HTML reduction pipeline for each portal | Agent with a web-fetch Tool. Portal-specific rules are text in the context. No custom code for each portal |
| No PDF fallback when anti-bot protection blocks access | PDF upload is supported natively (the Claude API can read PDFs directly) |
| No persistent "how should I work" context | Global + profile-specific context, like AGENT.md, persistent and editable |

Details: [[spec.md]], [[CONTEXT.md]], [[user-stories.md]].

## Goal

A solid spec for a web app. The app copies the reference Obsidian workflow 1:1. It reuses the transparent evaluation model from the old spec of the predecessor project.

## End criterion

The spec is complete enough to start phase 1 development (chat interface + Context Files + manual/URL ingestion + evaluation + list). No open questions about the base architecture should remain.

## Notes

- The reference Obsidian workflow holds the "working instructions for future sessions". These should take over the role of the profile context 1:1.
- Do not delete the old spec of the predecessor project. Its data model (criteria, grades, weighting, score formula) is kept. Only the ingestion and interaction layer changes completely.
- The test catalog is deliberately not created yet. Create it only after the base idea is validated (see Next steps).
- ADRs for the chat/context/agent architecture are created: [[docs/adr/README|docs/adr/]] (6 ADRs, own numbering; ADR 0003/0004 replace earlier decisions of the predecessor project).
- 2026-07-29: Switching the POC provider to OpenCode was considered and rejected. Feasibility (reliable multi-turn Tool use, native PDF input) favors Claude. It stays with [[docs/adr/0006-single-provider-for-agent-mvp|ADR 0006]].
- Deployment to a remote Docker host (image as a file, no registry): [[deployment]]
- Server operations (configuration, start/stop, backup, using the MCP server): [[operations]]

## Next steps

- [ ] Spec review focused on the Tool definitions for the Agent (spec/00_infrastructure_setup/00-03-llm-agent-integration.md)
- [x] Decide: build a standalone chat-agent loop ourselves, or use the Anthropic Agent SDK/Tool-Runner — decided: own loop that does not depend on a provider, see [[docs/adr/0007-agent-loop-provider-independent]]
- [x] Check whether uploading a PDF directly to the Claude API fully replaces the pymupdf workaround from the Obsidian workflow — decided, see [[docs/adr/0005-pdf-native-document-input]]
- [x] Create ADRs for the central architecture decisions (docs/adr/)
- [ ] Set up the test catalog
- [ ] Re-evaluate the LM Studio / OpenCode provider implementation after MVP validation (see [[docs/adr/0006-single-provider-for-agent-mvp]])
