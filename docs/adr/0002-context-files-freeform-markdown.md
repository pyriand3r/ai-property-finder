---
type: adr
title: "Context Files are freeform Markdown, not structured settings"
project: property-finder-obs-style
status: accepted
created: 2026-07-28
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. It affects [[../../spec/01_mvp/01-02-context-files]] and [[../../spec/00_infrastructure_setup/00-02-database-schema]].

## Context

Global and Profile Context must hold things like "bei schwedischen Objekten immer Wasser/Abwasser prüfen" (means: for Swedish properties, always check water/sewage) or "Booli → folge dem Läs-mer-hos-mäklaren-Link" (means: on Booli, follow the "read more at the broker" link).

A structured settings model would have a typed rule table, one row per instruction, and dropdowns for scope and trigger. It would be easier to query. The UI could also check input. But it would force us to predict every shape an instruction can take.

The reference Obsidian workflow has `AGENT.md`-style instructions. They freely mix procedural steps, formatting rules, and one-off exceptions, all in prose. That mix makes them easy to write and easy for an LLM to follow.

## Decision

`context.content_md` is one Markdown text field per Context File ([[../../spec/00_infrastructure_setup/00-02-database-schema#context]]). There is no structured rule schema and there are no typed trigger or condition fields. The Agent's system prompt includes the text word for word. History works by saving a snapshot of the whole text on every change (`context_revision`). It does not diff structured rows.

## Alternatives considered

- **Structured rule table** (one row per instruction, `trigger_domain`, `action_type`, …) — rejected. It forces every instruction into a fixed shape. The real instructions of the reference workflow do not fit one shape. A rigid schema repeats the mistake of the old spec of the predecessor project, which used rigid Listing forms. Only the entity is different.
- **Structured Portal Rule table with a freeform fallback field** — rejected as too early. Nothing shows yet that the freeform text needs structured indexing (see [[0003-portal-rules-as-context-text]]). We can add structure later if freeform Context becomes hard to manage at volume.

## Consequences

- A query like "show all profiles with a rule for domain X" needs full-text search over `content_md`. This is fine for a single-user app with a few profiles.
- The editor is just a Markdown textarea ([[../../spec/01_mvp/01-02-context-files#Editor]]). It is cheap to build. It matches how people write in the reference workflow (editing an Obsidian note).
- Token cost: the full Context File goes to the model on every Agent turn. There is no selection of only "relevant" rules. This is fine while Context Files stay small. If they grow, it is an open question in [[../../spec/01_mvp/01-02-context-files#Open Questions / TBD]].
