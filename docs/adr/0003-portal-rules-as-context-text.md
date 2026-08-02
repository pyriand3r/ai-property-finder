---
type: adr
title: "Portal-specific handling lives as natural-language rules in Context, not scraper code"
project: property-finder-obs-style
status: accepted
created: 2026-07-28
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — supersedes [[../../../dev-property-finder/docs/adr/0007-scraping-anti-bot-strategy|dev-property-finder ADR 0007]] and [[../../../dev-property-finder/docs/adr/0008-html-reduction-pipeline|ADR 0008]] as the mechanism for portal-specific behaviour. Affects [[../../spec/01_mvp/01-03-listing-ingestion-via-chat]], [[../../CONTEXT#Portal Rule]].

## Context

The old dev-property-finder spec handled portal differences in code: a per-portal scraper module, a bot-detection escalation ladder (ADR 0007), and a bespoke HTML-reduction pipeline that fed structured extraction (ADR 0008). That's real engineering weight for a single-user tool, and it still couldn't express something as simple as "on Booli, the real details are one click away, on the broker's own page" — that fact lived in the human's head, not in any scraper config, which is exactly why the reference Obsidian workflow writes it down as a plain instruction in `00_readme.md` instead.

## Decision

Portal-specific knowledge — which secondary link to follow, which fields typically hide on a broker page, known quirks of a domain — is written as plain-language Portal Rules inside Global or Profile Context (see [[../../CONTEXT#Portal Rule]]), interpreted by the Agent at runtime via `fetch_url`'s returned links and its own reading of the page. No portal gets its own Go module. `fetch_url` stays generic: render, reduce, return text + image URLs + outbound links (see [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]]) — it does not know what a "Booli listing" is.

Anti-bot handling is *not* re-engineered as a hardening ladder up front. When `fetch_url` hits a wall, the Agent reports it and asks for a PDF ([[../../spec/01_mvp/01-03-listing-ingestion-via-chat#Failure Modes]]) — the same "human clears the wall" fallback that dev-property-finder ADR 0007 eventually landed on as its universal Tier D, just promoted here to the *default* behaviour instead of the last resort of a four-tier ladder.

## Alternatives considered

- **Port the Tier 0–2 hardening ladder from ADR 0007 as-is** — rejected for MVP: that ladder exists to reduce how often a human has to intervene at scale (Phase 3 scheduled scraping). This project's MVP is chat-driven, one Source at a time, with the user present in the loop already — the case the ladder optimises for doesn't exist yet. Revisit if/when Phase 3 (Automated Recurring Search, [[../../spec.md#Phase 3]]) is built.
- **Structured Portal Rule table** (domain → follow-link selector, in code or DB) — rejected: same reasoning as [[0002-context-files-freeform-markdown]]. A selector-based rule breaks the moment a portal changes its markup; a natural-language instruction interpreted by the Agent degrades gracefully instead.

## Consequences

- Adding support for a new portal is a Context edit, not a deploy — directly matches how the reference workflow already scales (the user just writes a new paragraph in `00_readme.md`)
- No compile-time guarantee that a Portal Rule is being followed correctly — the Agent can misread or ignore an instruction; the user catches this the same way they catch any Agent mistake, via the mandatory form fallback ([[0001-chat-first-with-form-fallback]])
- `internal/scraper/`'s field-extraction and HTML-reduction pipeline (dev-property-finder ADR 0008) is dropped entirely, not ported — see [[../../spec/00_infrastructure_setup/00-01-tech-stack#What's Dropped vs. dev-property-finder]]
- Deferred, not solved: reliable automated access to heavily bot-protected portals at scale. Explicitly out of scope until Phase 3 needs it.
