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

Accepted — affects [[../../spec/01_mvp/01-02-context-files]], [[../../spec/00_infrastructure_setup/00-02-database-schema]].

## Context

Global and Profile Context need to hold things like "bei schwedischen Objekten immer Wasser/Abwasser prüfen" or "Booli → folge dem Läs-mer-hos-mäklaren-Link." A structured settings model (typed rule table, one row per instruction, dropdowns for scope/trigger) would be more queryable and would let the UI validate input. It would also require anticipating every shape an instruction can take — the reference workflow's actual `AGENT.md`-style instructions ([[../../../immobilienplan-schweden-portugal-oesterreich/schweden/00_readme#⚙️ Arbeitsanweisungen für zukünftige Sessions]]) mix procedural steps, formatting rules, and one-off exceptions freely, in prose, and that mix is exactly what makes them easy to write and easy for an LLM to follow.

## Decision

`context.content_md` is a single Markdown text field per Context File ([[../../spec/00_infrastructure_setup/00-02-database-schema#context]]) — no structured rule schema, no typed trigger/condition fields. The Agent's system prompt includes it verbatim. History is handled by snapshotting the whole text on every change (`context_revision`), not by diffing structured rows.

## Alternatives considered

- **Structured rule table** (one row per instruction, `trigger_domain`, `action_type`, …) — rejected: forces every instruction through a predefined shape; the reference workflow's actual instructions don't fit one shape, and a rigid schema is the same mistake the old dev-property-finder spec made with rigid Listing forms, just moved to a different entity.
- **Structured Portal Rule table with a freeform fallback field** — rejected as premature: nothing today proves the freeform text needs structured indexing yet (see [[0003-portal-rules-as-context-text]]); adding structure can happen later if freeform Context proves hard to manage at volume.

## Consequences

- No query like "show all profiles with a rule for domain X" is possible without full-text search over `content_md` — acceptable for a single-user app with a handful of profiles.
- The editor is just a Markdown textarea ([[../../spec/01_mvp/01-02-context-files#Editor]]) — cheap to build, matches the reference workflow's actual authoring experience (editing an Obsidian note).
- Token cost: the full Context File is sent on every Agent turn (no selective inclusion of "relevant" rules) — acceptable while Context Files stay small; flagged as an open question in [[../../spec/01_mvp/01-02-context-files#Open Questions / TBD]] if they grow.
