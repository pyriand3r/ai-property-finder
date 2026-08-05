---
type: reference
title: "Property Finder OBS-Style — Domain Glossary"
project: property-finder-obs-style
tags:
  - property-finder-obs-style
  - reference
  - domain
---

A single-user, self-hosted application for finding and evaluating real estate listings through a conversational AI agent. The user drives ingestion and evaluation through a Chat, not through forms. Extends the domain model of [[../dev-property-finder/CONTEXT|dev-property-finder/CONTEXT]] — Criterion, Grade, Score, Weight are unchanged; this glossary only defines what's new.

## Language

**Search Profile**:
Unchanged from [[../dev-property-finder/CONTEXT#Search Profile|dev-property-finder]] — a named, user-defined set of Criteria describing the desired property. Now additionally owns exactly one Profile Context.

**Chat Session**:
A persistent, ordered sequence of messages between the user and the Agent, scoped to exactly one Search Profile. Messages are stored durably — the Chat Session is not ephemeral browser state. Reopening a Search Profile resumes its Chat Session where it left off.
_Avoid_: Conversation (too generic), thread

**Agent**:
The LLM acting inside a Chat Session with access to a fixed set of Tools ([[#Tool]]). Distinct from the plain evaluation calls in [[../dev-property-finder/CONTEXT#Evaluation|dev-property-finder]] (`EvaluateListing`, `ParseSearchProfile`) — the Agent runs a multi-turn tool-use loop, not a single request/response call. The old single-shot Evaluation calls remain and are invoked by the Agent as a Tool, not replaced.
_Avoid_: Assistant, bot, model (model is the underlying LLM; Agent is the role it plays with tools attached)

**Tool**:
A typed function the Agent can call during a Chat turn (e.g. `fetch_url`, `extract_images`, `save_listing`, `update_profile_context`). Tool results are fed back to the Agent within the same turn before it produces a reply. Full catalogue: [[spec/00_infrastructure_setup/00-03-llm-agent-integration]].
_Avoid_: Function, action, command

**Context File**:
A persistent, user- and Agent-editable Markdown text that steers Agent behaviour — the app's equivalent of an `AGENT.md`. Two scopes exist: Global Context and Profile Context. Neither is a prompt template with variable substitution — both are plain instructional text prepended to the Agent's system prompt.
_Avoid_: Prompt, system prompt (the Context File is the *content*; the system prompt is the mechanism that includes it), config

**Global Context**:
The single, app-wide Context File. Partially pre-seeded by the application (e.g. default image-extraction approach, default table/rating format, star-rating convention) and freely extended by the user or the Agent (with user confirmation — see [[spec/01_mvp/01-02-context-files]]). Applies to every Search Profile's Agent turns.
_Avoid_: Global config, system instructions

**Profile Context**:
A Context File owned by exactly one Search Profile. Holds profile-specific instructions the user would otherwise repeat every session — e.g. "for Swedish listings, always check water/sewage and follow the Booli broker link." Directly analogous to the "Arbeitsanweisungen für zukünftige Sessions" section maintained today in [[../immobilienplan-schweden-portugal-oesterreich/schweden/00_readme]]. Appended after the Global Context in the Agent's system prompt; more specific instructions here override more general ones in the Global Context.
_Avoid_: Profile config, profile notes (see [[#Listing Note]] — a different, per-Listing concept)

**Portal Rule**:
A Context File fragment (living in Global or Profile Context — not a separate structured table) describing how to handle listings from a specific domain: which secondary link to follow (e.g. "Läs mer hos mäklaren"), what fields typically hide only on the broker page, anti-bot quirks. Interpreted by the Agent at runtime via natural language, not compiled into portal-specific scraper code — this is the key architectural break from [[../dev-property-finder/docs/adr/0007-scraping-anti-bot-strategy|dev-property-finder ADR 0007]] and [[../dev-property-finder/docs/adr/0008-html-reduction-pipeline|ADR 0008]].
_Avoid_: Scraper config, portal (Portal remains the entity name for a *search source* in the old Phase-3 sense, if that phase is ever ported — Portal Rule is purely a context-file concept)

**Source**:
What the Agent was given to work from for one Listing: a URL, an uploaded PDF, or an uploaded image. A Listing always has exactly one Source, recorded on ingestion (see [[../dev-property-finder/CONTEXT#Import|dev-property-finder Import]], which this specialises).
_Avoid_: Input, origin

**Listing Document**:
The long-form, human-readable write-up the Agent produces per Listing — original-language excerpt, German translation, criteria evaluation table, pros/cons versus other Listings in the profile, broker-page details. Stored as structured Markdown in `listing.document_md` (see [[spec/00_infrastructure_setup/00-02-database-schema]]) and rendered in the Listing detail view. This is the direct counterpart of today's per-Inserat `.md` file in the Obsidian vault — the difference is it lives inside one `listing` row instead of being the row.
_Avoid_: Description (Description remains the shorter structured field inherited from the old model — see [[../dev-property-finder/CONTEXT#Listing|dev-property-finder Listing]]), notes (see [[#Listing Note]] — a separate, user-authored concept)

**Listing Note**:
A freeform, timestamped note the user writes about a Listing itself — "called the agent, viewing Saturday," "neighbor mentioned the roof was redone in 2019." Any number per Listing, each independently created/edited/deleted. Stored in `listing_note` (see [[spec/00_infrastructure_setup/00-02-database-schema]]) and shown in the Listing detail view as individual cards ([[spec/01_mvp/01-11-listing-notes]]).
_Avoid_: Notes (ambiguous — see below), Verkäufer-Notizen/seller notes (that's `listing.seller_notes`, a single field about the seller/contact person, not this)

**Ranking**:
The user-facing, ordered view of all "active" Listings in a Search Profile, driven by Score. Each Listing carries an AI-written per-listing one-liner (`listing.kurzfazit`), and the profile holds an overall comparison summary (`search_profile.ranking_summary`). Both are regenerated on user request (chat instruction or button), not automatically. Mirrors the Rangliste section of the reference workflow's `00_readme.md`.
_Avoid_: List view (List view is the plain sortable/filterable table, unchanged from the old model — Ranking is the curated, narrated variant)

**Photo Set**:
The images the Agent gathered or was given for a Listing, each tagged by the Agent with a category (exterior, interior, floor plan, outbuilding, other) where determinable. One image is marked as the title image. Extends `listing_image` ([[../dev-property-finder/CONTEXT#Listing|dev-property-finder]]) with a `category` field — see [[spec/00_infrastructure_setup/00-02-database-schema]].
_Avoid_: Gallery, media

**Follow-Up Instruction**:
A user message inside an existing Listing's Chat context asking the Agent to revisit or extend a specific Listing ("check if this one has a garage", "re-read the broker page, they added a floor plan"). Not a new Import — it updates the existing Listing and triggers re-evaluation, same as a manual field edit in the old model ([[../dev-property-finder/CONTEXT#Evaluation|dev-property-finder Evaluation]]).
_Avoid_: Edit request, correction

## Carried over unchanged from dev-property-finder

Criterion, Objective/Subjective Criterion, Weight, Evaluation, Grade, Score, Status, Letztkontakt, Rejection, Property. See [[../dev-property-finder/CONTEXT]] for definitions — not restated here to avoid drift between two copies.

**Explicitly dropped**: Proposal (the auto-generated Criteria set after a Rejection). In the chat-driven model, the equivalent happens conversationally — the user tells the Agent why a Listing doesn't fit, and the Agent proposes Profile Context or Criteria changes directly in the chat reply, which the user accepts by saying so or by editing the Context File / Criteria list. No separate pending-Proposal entity is needed. Portal (the Phase-3 automated recurring search source) is deferred, not dropped — out of scope for this spec's MVP, see [[spec.md#Development Phases]].
