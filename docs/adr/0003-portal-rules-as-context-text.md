---
type: adr
title: "Portal-specific handling lives as plain-language rules in Context, not scraper code"
project: property-finder-obs-style
status: accepted
created: 2026-07-28
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. It replaces two earlier decisions in the predecessor project, about its scraping anti-bot strategy and its HTML reduction pipeline, as the way to handle portal-specific behaviour. It affects [[../../spec/01_mvp/01-03-listing-ingestion-via-chat]] and [[../../CONTEXT#Portal Rule]].

## Context

The old spec of the predecessor project handled portal differences in code. It had a scraper module per portal. It had a bot-detection escalation ladder. It had a custom HTML-reduction pipeline that fed structured extraction. That is a lot of engineering for a single-user tool. It still could not express a simple fact like "on Booli, the real details are one click away, on the broker's own page." That fact lived in the human's head, not in any scraper config. This is why the reference Obsidian workflow writes it down as a plain instruction.

## Decision

Portal-specific knowledge is written as plain-language Portal Rules inside Global or Profile Context (see [[../../CONTEXT#Portal Rule]]). Examples of this knowledge: which secondary link to follow, which fields usually hide on a broker page, and known quirks of a domain. The Agent interprets the rules at runtime. It uses the links that `fetch_url` returns and its own reading of the page. No portal gets its own Go module. `fetch_url` stays generic. It renders, reduces, and returns text, image URLs, and outbound links (see [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]]). It does not know what a "Booli listing" is.

We do *not* build an anti-bot hardening ladder up front. When `fetch_url` hits a wall, the Agent reports it and asks for a PDF ([[../../spec/01_mvp/01-03-listing-ingestion-via-chat#Failure Modes]]). This is the "human clears the wall" fallback. The predecessor project ended up with it as its universal last tier. Here it becomes the *default* behaviour, not the last resort of a four-tier ladder.

## Alternatives considered

- **Port the Tier 0–2 hardening ladder from the predecessor project as-is** — rejected for MVP. That ladder reduces how often a human must step in at scale (Phase 3 scheduled scraping). The MVP here is chat-driven, one Source at a time, and the user is already in the loop. The case the ladder is built for does not exist yet. Revisit it if Phase 3 (Automated Recurring Search, [[../../spec.md#Phase 3]]) is built.
- **Structured Portal Rule table** (domain → follow-link selector, in code or DB) — rejected. The reasoning is the same as in [[0002-context-files-freeform-markdown]]. A selector-based rule breaks when a portal changes its markup. A plain-language instruction that the Agent interprets degrades gracefully.

## Consequences

- Adding support for a new portal is a Context edit, not a deploy. This matches how the reference workflow already scales: the user just writes a new paragraph in its instructions.
- There is no compile-time guarantee that a Portal Rule is followed correctly. The Agent can misread or ignore an instruction. The user catches this like any other Agent mistake, through the required form fallback ([[0001-chat-first-with-form-fallback]]).
- The field-extraction and HTML-reduction pipeline of `internal/scraper/` (from the predecessor project) is dropped completely, not ported. See [[../../spec/00_infrastructure_setup/00-01-tech-stack#What's Dropped vs. the Predecessor Project]].
- Deferred, not solved: reliable automated access to heavily bot-protected portals at scale. This is explicitly out of scope until Phase 3 needs it.
