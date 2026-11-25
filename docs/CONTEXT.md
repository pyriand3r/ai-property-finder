---
type: reference
title: "Property Finder OBS-Style — Domain Glossary"
project: property-finder-obs-style
tags:
  - property-finder-obs-style
  - reference
  - domain
---

A single-user, self-hosted app. It helps you find and judge real estate listings through a conversational AI agent. The user adds and evaluates listings in a Chat, not in forms. It builds on the domain model of the predecessor project. Criterion, Grade, Score and Weight stay the same. This glossary only defines what is new.

## Language

**Search Profile**:
Same as in the predecessor project. It is a named set of Criteria, defined by the user, that describes the property they want. It now also owns exactly one Profile Context.

**Chat Session**:
A saved, ordered list of messages between the user and the Agent. It belongs to exactly one Search Profile. Messages are stored for good. The Chat Session is not temporary browser state. When you reopen a Search Profile, its Chat Session continues where it stopped.
_Avoid_: Conversation (too generic), thread

**Agent**:
The LLM that works inside a Chat Session. It can use a fixed set of Tools ([[#Tool]]). It is different from the plain evaluation calls in the predecessor project (`EvaluateListing`, `ParseSearchProfile`). The Agent runs a multi-turn tool-use loop. It is not one request and one response. The old single-shot Evaluation calls still exist. The Agent calls them as a Tool. They are not replaced.
_Avoid_: Assistant, bot, model (model is the underlying LLM; Agent is the role it plays with tools attached)

**Tool**:
A typed function that the Agent can call during a Chat turn (for example `fetch_url`, `extract_images`, `save_listing`, `update_profile_context`). The Agent gets the Tool results back in the same turn, before it writes a reply. Full list: [[spec/00_infrastructure_setup/00-03-llm-agent-integration]].
_Avoid_: Function, action, command

**Context File**:
A saved Markdown text that steers how the Agent behaves. The user and the Agent can both edit it. It is the app's version of an `AGENT.md`. There are two scopes: Global Context and Profile Context. Neither is a prompt template with variables. Both are plain instruction text added at the start of the Agent's system prompt.
_Avoid_: Prompt, system prompt (the Context File is the *content*; the system prompt is the mechanism that includes it), config

**Global Context**:
The one Context File for the whole app. The app fills part of it in advance (for example the default way to extract images, the default table and rating format, and the star-rating convention). The user or the Agent can add to it freely. The Agent needs user confirmation first (see [[spec/01_mvp/01-02-context-files]]). It applies to the Agent's turns in every Search Profile.
_Avoid_: Global config, system instructions

**Profile Context**:
A Context File that belongs to exactly one Search Profile. It holds instructions for that profile that the user would otherwise repeat in every session. One example: "for Swedish listings, always check water/sewage and follow the Booli broker link." It works like the "Arbeitsanweisungen für zukünftige Sessions" section (means: "work instructions for future sessions") that is kept by hand today in the reference Obsidian workflow. It is added after the Global Context in the Agent's system prompt. When both say something different, the more specific instruction here wins over the general one in the Global Context.
_Avoid_: Profile config, profile notes (see [[#Listing Note]] — a different, per-Listing concept)

**Portal Rule**:
A piece of a Context File (in Global or Profile Context, not in a separate structured table). It says how to handle listings from one specific domain. It can say which second link to follow (for example "Läs mer hos mäklaren", Swedish for "read more at the broker"). It can say which fields are often only on the broker page. It can describe anti-bot quirks. The Agent reads it at runtime as natural language. It is not compiled into scraper code for each portal. This is the key break in architecture from the scraping anti-bot strategy and the HTML reduction pipeline of the predecessor project.
_Avoid_: Scraper config, portal (Portal remains the entity name for a *search source* in the old Phase-3 sense, if that phase is ever ported — Portal Rule is purely a context-file concept)

**Source**:
What the Agent got to work from for one Listing: a URL, an uploaded PDF, or an uploaded image. A Listing always has exactly one Source. The app records it when the Listing is added. This makes the Import of the predecessor project more specific.
_Avoid_: Input, origin

**Listing Document**:
The long, human-readable write-up that the Agent makes for each Listing. It holds an excerpt in the original language, a German translation, a table that evaluates the criteria, pros and cons compared with the other Listings in the profile, and details from the broker page. It is stored as structured Markdown in `listing.document_md` (see [[spec/00_infrastructure_setup/00-02-database-schema]]). The Listing detail view shows it. It is the direct match for today's `.md` file for each Inserat (German: listing) in the reference Obsidian workflow. The difference is that it lives inside one `listing` row. It is not the row itself.
_Avoid_: Description (Description remains the shorter structured field inherited from the old model), notes (see [[#Listing Note]] — a separate, user-authored concept)

**Listing Note**:
A free-form note with a timestamp. The user writes it about a Listing itself, for example "called the agent, viewing Saturday" or "neighbor mentioned the roof was redone in 2019." A Listing can have any number of them. Each one can be created, edited and deleted on its own. It is stored in `listing_note` (see [[spec/00_infrastructure_setup/00-02-database-schema]]). The Listing detail view shows each one as its own card ([[spec/01_mvp/01-11-listing-notes]]).
_Avoid_: Notes (ambiguous — see below), Verkäufer-Notizen/seller notes (that's `listing.seller_notes`, a single field about the seller/contact person, not this)

**Ranking**:
The ordered view of all "active" Listings in a Search Profile, driven by Score. The user sees this view. Each Listing has a one-line summary written by the AI (`listing.kurzfazit`). The profile also holds one overall comparison summary (`search_profile.ranking_summary`). Both are made again when the user asks, by a chat instruction or a button. They are not updated automatically. It matches the Rangliste (German: ranking list) section of the reference Obsidian workflow.
_Avoid_: List view (List view is the plain sortable/filterable table, unchanged from the old model — Ranking is the curated, narrated variant)

**Photo Set**:
The images that the Agent collected or received for a Listing. Where it can tell, the Agent tags each image with a category (exterior, interior, floor plan, outbuilding, other). One image is marked as the title image. It extends the `listing_image` of the predecessor project with a `category` field. See [[spec/00_infrastructure_setup/00-02-database-schema]].
_Avoid_: Gallery, media

**Follow-Up Instruction**:
A user message in the Chat context of an existing Listing. It asks the Agent to look at that Listing again or to add to it ("check if this one has a garage", "re-read the broker page, they added a floor plan"). It is not a new Import. It updates the existing Listing and starts a new evaluation. This is the same as a manual field edit in the old model.
_Avoid_: Edit request, correction

**Proposed Criterion**:
A Criterion with `status = 'proposed'`. The Agent (chat) or an MCP client suggested it, and the user has not approved it yet. It is shown in the Criteria tab with a "Vorgeschlagen" badge and is ignored by evaluation, Score and ranking until the user approves ("Freischalten"). A Criterion with `status = 'active'` is in use. See [[adr/0024-criterion-proposal-status]].
_Avoid_: Draft criterion, pending criterion

**Evaluation Brief**:
What an external grader gets from the MCP tool `get_evaluation_brief`: the grading instruction, the active Criteria with their IDs, the Listing, both Context Files and the user-confirmed fields. The grader answers with `submit_evaluation`. The server checks the Grades and computes the Score. See [[spec/01_mvp/01-14-external-evaluation-via-mcp]] and [[adr/0026-external-evaluation-via-mcp]].

**Hazard Rating**:
A whole number from 1 (very low) to 5 (very high). It says how exposed one Hazard Region is to one Hazard Type. **Higher is worse**, the opposite direction of the Score. It belongs to the region, not to a Listing, and it is never an input to the Score, the ranking or the evaluation. Ratings of different hazard types come from different methods and are not comparable. A hazard with no source data for a region has no Hazard Rating. See [[spec/01_mvp/01-13-hazard-ratings]] and [[adr/0025-hazard-ratings-region-level-static-data]].
_Avoid_: Risk score, Naturrisiko-Score (means: natural-risk score), hazard grade (a Grade is the A-F mark of a Criterion)

**Hazard Type**:
One of four natural hazards: `wildfire`, `flood`, `earthquake`, `landslide`. Each Hazard Type has its own Hazard Source per country.
_Avoid_: Peril, risk category, disaster type

**Hazard Region**:
The geographic unit a Hazard Rating is given for. For Portugal it is the municipality (*concelho*). A Hazard Region has a key like `PT-1106`: the country code, a dash and the official national code (for Portugal the 4-digit DICO code). Names are not unique, so the key is the identity. It is the place of many Listings, not the place of one Listing.
_Avoid_: Location (a Listing's own address and coordinates), area, zone (a seismic zone is a value inside the source data)

**Hazard Source**:
The published dataset that a Hazard Rating was derived from: publisher, URL, licence, version, retrieval date and a `method` text. The `method` says how the dataset's own values became the 1-5 scale, and which step is the app's own interpretation. A Source in the sense of [[#Source]] is something else: what the Agent read for one Listing.
_Avoid_: Provider (the LLM provider), data feed

**Hazard Dataset**:
The static JSON file of one country, embedded in the backend binary, that holds its Hazard Sources, Hazard Regions and Hazard Ratings. The backend copies it into the database at boot. Nothing is fetched at runtime. To change a rating, replace the file and release.
_Avoid_: Seed data, hazard import, hazard feed

**Hazard Source Catalogue**:
The declarative list, one embedded JSON file per country, of the hazard data sources known for it. An entry is `bundled` (its data ships in the Hazard Dataset) or `candidate` (a known lead, not ingested). See [[spec/01_mvp/01-15-hazard-source-catalogue]] and [[adr/0027-hazard-source-catalogue-and-suggestions]].
_Avoid_: Source list, source registry

**Source Suggestion**:
A hazard data source the Agent or an MCP client proposed through `suggest_hazard_source`, stored as `proposed`. The user approves or rejects it in the Global Context view. Approval clears the source for ingestion in a later release. It does not create ratings.
_Avoid_: Source proposal (a Criterion has a proposal, see [[#Proposed Criterion]]), source request

**Portal Search**:
One run of the `search_portal` Tool over a portal search-result URL (one or more result pages). It returns candidate Listing links, split into new and known (a Listing of the profile already has that `source_url`). It creates no Listing. It is stored as a `task` row of type `portal.search` and continued after a restart. See [[spec/01_mvp/01-16-portal-search]] and [[adr/0028-portal-search-task-persistence]].
_Avoid_: Crawl, scrape, Portal (a Portal Rule is a Context File fragment, see [[#Portal Rule]])

**Saved Search**:
A portal search-result URL (with the `search_portal` settings) that the app searches on a schedule, "every N hours". Each run is a Portal Search. Links that are new since the last run can be handed to the Agent, which loads them as Listings in the profile's active Chat Session. Stored in table `saved_search`. See [[spec/01_mvp/01-17-scheduled-searches]] and [[adr/0029-scheduled-saved-searches]].
_Avoid_: Alert, watch, crawler job

## Carried over unchanged from the predecessor project

Criterion, Objective/Subjective Criterion, Weight, Evaluation, Grade, Score, Status, Letztkontakt, Rejection, Property. They keep the meaning they had in the predecessor project. They are not repeated here, so that two copies do not drift apart.

**Explicitly dropped**: Proposal (the set of Criteria that was made automatically after a Rejection). In the chat-driven model, the same thing happens in conversation. The user tells the Agent why a Listing does not fit. The Agent then suggests changes to the Profile Context or Criteria directly in its chat reply. The user accepts by saying so, or by editing the Context File or the Criteria list. No separate pending-Proposal entity is needed. Portal (the Phase-3 source for automated recurring search) is postponed, not dropped. It is out of scope for the MVP of this spec, see [[spec.md#Development Phases]].
