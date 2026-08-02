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

A client-server web application that helps a user find and evaluate real estate listings through a conversational AI Agent. The Agent — not a form wizard — is the primary way listings enter and get refined in the system. It reads listing pages or uploaded PDFs, follows portal-specific secondary links, extracts photos, writes a structured Listing Document, and evaluates each Listing against transparent, weighted, per-Criterion Grades (A–F). Terminology: [[CONTEXT.md]].

This spec supersedes the interaction model of [[../dev-property-finder/spec|dev-property-finder]] while reusing its Criteria/Grade/Score/Weight data model, which is unrelated to the ingestion problem it got wrong. See [[00_readme.md#Warum der alte Ansatz falsch war]] for the diff.

---

## Tech Stack

| Layer      | Technology                                  | Notes |
|------------|----------------------------------------------|-------|
| Backend    | Go (Golang)                                   | Unchanged from dev-property-finder |
| Frontend   | Vue.js 3 (Composition API) + TypeScript       | Unchanged |
| API        | REST + dedicated Chat streaming endpoint | Task SSE reused for background progress; Chat streams over `/api/chat/stream` |
| Map        | OpenStreetMap (OSM / OpenLayers)              | Unchanged |
| AI         | Claude API (agentic tool use, incl. native PDF input) | Sole MVP provider. LM Studio and OpenCode kept as planned swap targets behind the provider interface, deferred post-MVP — see [[docs/adr/0006-single-provider-for-agent-mvp]] |
| Database   | PostgreSQL                                    | Unchanged |

Full detail: [[spec/00_infrastructure_setup/00-01-tech-stack]].

---

## Core Features

### Search Profile Management

Search Profiles are created, renamed, and archived through a plain form — a one-time structural action, not something the Chat needs to be involved in. Each profile is the parent of its own Chat, Context, Criteria, and Listings. See [[spec/01_mvp/01-08-profile-management]].

### Chat Interface

The primary screen per Search Profile is a chat window, not a listing form. The user pastes a URL, uploads a PDF, or types an instruction ("bewerte das", "vergleiche mit Kappmuren 2", "folge dem Makler-Link nochmal"). The Agent replies in the chat and, where relevant, its actions show up immediately in the Listing list/detail/map views next to the chat. See [[spec/01_mvp/01-01-chat-interface]].

### Context Files (Global + Profile)

Two Markdown text stores steer the Agent, directly analogous to a vault-wide `AGENT.md` plus a per-topic memory file:

- **Global Context**: app-wide rules, partially pre-seeded (photo handling, table format, star-rating scale), user- and Agent-extensible
- **Profile Context**: one per Search Profile, holds profile-specific instructions ("for Swedish listings, always check water/sewage")

Both are visible and directly editable by the user at any time; the Agent proposes additions after learning something durable from the conversation, but never silently overwrites without the user seeing the diff. See [[spec/01_mvp/01-02-context-files]].

### Listing Ingestion via Chat

Given a URL or an uploaded PDF, the Agent: fetches/reads the Source, follows a secondary link if the applicable Portal Rule (living in Context, not code) says so, extracts and downloads photos where possible, writes the Listing Document (original excerpt, German translation, structured fields), and evaluates it against the profile's Criteria. The user watches this happen turn by turn in the chat and can interrupt or redirect at any point. See [[spec/01_mvp/01-03-listing-ingestion-via-chat]].

### Criteria & Transparent Scoring

Unchanged model from dev-property-finder: AI-proposed Criteria, each Objective or Subjective, each with a user-adjustable Weight (1–5), each Listing gets an A–F Grade per Criterion with a written reasoning, and a computed 0–100 overall Score (`sum(grade_value * weight) / sum(weight)`). New: Criteria can be added/edited/removed directly by the user (no longer parse-only), and can also be added conversationally ("also grade for proximity to water"). See [[spec/01_mvp/01-04-criteria-and-scoring]].

### List, Ranking & Map

- **List view**: sortable/filterable table of all Listings in a profile — unchanged from dev-property-finder
- **Ranking**: curated, narrated view mirroring the Obsidian `00_readme.md` Rangliste — ordered by Score, each row carries a short AI rationale, plus a "Kurzfazit" summary regenerated on request
- **Map view**: OSM map with one marker per Listing with coordinates, colour-coded by Status, popup with key facts — same as [[../immobilienplan-schweden-portugal-oesterreich/schweden/00_karte.html]]

See [[spec/01_mvp/01-05-ranking-and-list-view]] and [[spec/01_mvp/01-06-map-view]].

### Manual Review & Correction

Every field the Agent populates is visible and editable in a plain detail form (not only through chat) — the user must be able to trace and correct anything without going back through the Agent. Editing a field marks it as user-confirmed and triggers re-evaluation of that Listing, same trigger semantics as the old model. See [[spec/01_mvp/01-07-manual-review-and-edit]].

---

## Requirements

- Single-user application, private use in local network — unchanged from dev-property-finder
- No authentication required
- Deployment via Docker Compose
- All code, comments, API fields, and documentation in English; UI language German
- AI integration centres on Claude API's agentic tool-use + native PDF input; provider abstraction kept but LM Studio / OpenCode support deferred (see Tech Stack)
- Every Agent-written field remains user-editable; no field is chat-only

---

## Development Phases

### Phase 1 — MVP
- Search Profile management (create/rename/archive via form)
- Global + Profile Context Files (view/edit, app-seeded defaults)
- Chat interface scoped to one Search Profile, with the core ingestion Tools (fetch URL, read uploaded PDF, extract images, save/update Listing)
- Criteria: AI-proposed + manually editable, Weight adjustment, A–F Grade + reasoning, computed Score
- List view + manual field editing + re-evaluation on edit
- Ranking view with AI rationale + Kurzfazit
- Map view

### Phase 2 — Portal Rules & Multi-Source Depth
- Structured surfacing of Portal Rules inside Context Files (UI affordance to add/edit a rule for a domain, still stored as text)
- Multi-hop following (secondary + tertiary links, e.g. broker page → floor-plan PDF)
- Natural disaster risk rating per region, ported from [[../dev-property-finder/spec#Natural Disaster Risk Rating|dev-property-finder Phase 2]]

### Phase 3 — Automated Recurring Search *(carried over, still deferred)*
- Scheduled Chat-driven search runs against saved portal search-result URLs, same scope as [[../dev-property-finder/spec#Phase 3|dev-property-finder Phase 3]], reimplemented as an Agent Tool (`search_portal`) instead of a custom scraper module
