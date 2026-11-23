---
type: project-spec
created: 2026-07-28
title: Property Finder OBS-Style — Project Spec
project: property-finder-obs-style
status: ready
updated: 2026-07-28
tags:
  - property-finder-obs-style
  - spec
  - architecture
  - features
---

## Overview

This is a client-server web app. It helps a user find and judge real estate listings through a conversational AI Agent. The Agent is the main way listings enter the system and get improved. A form wizard is not. The Agent reads listing pages or uploaded PDFs. It follows portal-specific second links. It extracts photos. It writes a structured Listing Document. It evaluates each Listing against clear, weighted Grades (A–F), one for each Criterion. Terms are explained in [[CONTEXT.md]].

This spec replaces the interaction model of the predecessor project. It reuses the Criteria/Grade/Score/Weight data model of that project. That model is not related to the ingestion problem the old project got wrong. See [[00_readme.md#Why the old approach was wrong]] for the differences.

---

## Tech Stack

| Layer      | Technology                                  | Notes |
|------------|----------------------------------------------|-------|
| Backend    | Go (Golang)                                   | Same as the predecessor project |
| Frontend   | Vue.js 3 (Composition API) + TypeScript       | Same |
| API        | REST + dedicated Chat streaming endpoint | Task SSE is reused for background progress. Chat streams over `/api/chat/stream` |
| Map        | OpenStreetMap (OSM / OpenLayers)              | Same |
| AI         | Claude API (agentic tool use, incl. native PDF input) | The only provider in the MVP. LM Studio and OpenCode are kept as planned swap targets behind the provider interface. They are postponed until after the MVP — see [[docs/adr/0006-single-provider-for-agent-mvp]] |
| Database   | PostgreSQL                                    | Same |

Full detail: [[spec/00_infrastructure_setup/00-01-tech-stack]].

---

## Core Features

### Search Profile Management

The user creates, renames and archives Search Profiles with a plain form. This is a one-time structural action. The Chat does not need to be involved. Each profile is the parent of its own Chat, Context, Criteria and Listings. See [[spec/01_mvp/01-08-profile-management]].

### Chat Interface

The main screen for each Search Profile is a chat window. It is not a listing form. The user pastes a URL, uploads a PDF, or types an instruction ("bewerte das" (means: "rate this"), "vergleiche mit Kappmuren 2" (means: "compare with Kappmuren 2"), "folge dem Makler-Link nochmal" (means: "follow the broker link again")). The Agent answers in the chat. Where it matters, its actions show up at once in the Listing list, detail and map views next to the chat. See [[spec/01_mvp/01-01-chat-interface]].

### Context Files (Global + Profile)

Two Markdown text stores steer the Agent. They work like a vault-wide `AGENT.md` plus a memory file for each topic:

- **Global Context**: rules for the whole app. Part of it is filled in advance (photo handling, table format, star-rating scale). The user and the Agent can extend it
- **Profile Context**: one for each Search Profile. It holds instructions for that profile ("for Swedish listings, always check water/sewage")

The user can see and edit both at any time. The Agent suggests additions after it learns something lasting from the conversation. It never overwrites anything silently. The user always sees the diff. See [[spec/01_mvp/01-02-context-files]].

### Listing Ingestion via Chat

The user gives a URL or an uploaded PDF. Then the Agent does these steps:

1. It fetches or reads the Source.
2. It follows relevant secondary and tertiary links, including linked PDFs such as floor plans. Portal Rules in Context identify important links; clearly labelled listing documents can also be followed without a rule.
3. It extracts and downloads photos where possible.
4. It writes the Listing Document (original excerpt, German translation, structured fields).
5. It evaluates the Listing against the profile's Criteria.

The user watches this happen turn by turn in the chat. The user can stop or redirect it at any point. See [[spec/01_mvp/01-03-listing-ingestion-via-chat]].

### Criteria & Transparent Scoring

The model is the same as in the predecessor project:

- The AI proposes Criteria.
- Each Criterion is Objective or Subjective.
- Each Criterion has a Weight (1–5) that the user can change.
- Each Listing gets an A–F Grade for each Criterion, with written reasoning.
- The app computes an overall Score from 0 to 100: `sum(grade_value * weight) / sum(weight)`.

What is new: the user can add, edit and remove Criteria directly (they are no longer parse-only). Criteria can also be added in conversation ("also grade for proximity to water"). See [[spec/01_mvp/01-04-criteria-and-scoring]].

### List, Ranking & Map

- **List view**: a table of all Listings in a profile that you can sort and filter — same as in the predecessor project
- **Ranking**: a curated view with a story. It matches the Rangliste (German: ranking list) in the reference Obsidian workflow. It is ordered by Score. Each row has a short AI rationale. It also has a "Kurzfazit" summary (German: "short conclusion") that is made again on request
- **Map view**: an OSM map with one marker for each Listing that has coordinates. Markers are colour-coded by Status. A popup shows key facts — same as the map in the reference Obsidian workflow

See [[spec/01_mvp/01-05-ranking-and-list-view]] and [[spec/01_mvp/01-06-map-view]].

### Manual Review & Correction

The user can see and edit every field the Agent fills in. A plain detail form shows them, not only the chat. The user must be able to trace and fix anything without going back through the Agent. Editing a field marks it as user-confirmed. It also starts a new evaluation of that Listing. The trigger works the same way as in the old model. See [[spec/01_mvp/01-07-manual-review-and-edit]].

---

## Requirements

- Single-user app for private use in a local network — same as the predecessor project
- No authentication required
- Deployment with Docker Compose
- All code, comments, API fields and documentation are in English. The UI language is German
- The AI integration is built around the agentic tool use and native PDF input of the Claude API. The provider abstraction stays. Support for LM Studio and OpenCode is postponed (see Tech Stack)
- The user can edit every field the Agent writes. No field is chat-only

---

## Development Phases

### Phase 1 — MVP
- Search Profile management (create/rename/archive with a form)
- Global + Profile Context Files (view/edit, defaults filled in by the app)
- Chat interface for one Search Profile, with the core ingestion Tools (fetch URL, read uploaded PDF, extract images, save/update Listing)
- Criteria: proposed by the AI and editable by hand, Weight adjustment, A–F Grade + reasoning, computed Score
- List view + manual field editing + new evaluation after an edit
- Ranking view with AI rationale + Kurzfazit
- Map view

### Phase 2 — Portal Rules & Multi-Source Depth
- Show Portal Rules in a structured way inside Context Files (a UI control to add or edit a rule for a domain, still stored as text)
- Multi-hop following (second and third links, for example broker page → floor-plan PDF)
- Natural disaster risk rating for each region, taken over from the predecessor project (its Phase 2)

### Phase 3 — Automated Recurring Search *(carried over, still postponed)*
- Scheduled Chat-driven search runs against saved portal search-result URLs. The scope is the same as Phase 3 of the predecessor project. It is built again as an Agent Tool (`search_portal`) instead of a custom scraper module
