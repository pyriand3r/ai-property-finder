---
type: adr
title: "Chat is the primary interface; a plain form is a mandatory fallback"
project: property-finder-obs-style
status: accepted
created: 2026-07-28
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — foundational, affects [[../../spec/01_mvp/01-01-chat-interface]] and [[../../spec/01_mvp/01-07-manual-review-and-edit]].

## Context

The whole point of this project is that the old [[../../../dev-property-finder/spec|dev-property-finder]] spec got the interaction model wrong: formular-driven CRUD does not match how the workflow actually runs in practice ([[../../../immobilienplan-schweden-portugal-oesterreich/schweden/00_readme|reference workflow]]). The obvious fix is "make the Agent chat the only interface." That's a trap: a chat-only app is not auditable. If the Agent silently mis-extracts a price or picks the wrong coordinates, and the only way to see or fix that is another chat turn, the user has to *trust* the Agent completely — which is exactly the opposite of "der User muss alles gut nachvollziehen und auch manuell anpassen können" from the original brief.

## Decision

Chat is the primary and fastest path for getting work done — ingesting a Source, asking for comparisons, giving Follow-Up Instructions. But every structured field the Agent writes is also visible and directly editable in a plain form (the Listing detail view, [[../../spec/01_mvp/01-07-manual-review-and-edit]]), with no chat step required to reach or use it. The form is not a secondary/advanced view — it's load-bearing.

## Alternatives considered

- **Chat-only, no form** — rejected: no fallback when the Agent gets something wrong or unreachable (LLM outage), and no fast way to fix ten small errors across many Listings without ten chat turns.
- **Form-only with an AI "assist" button per field** — rejected: this is closer to the old dev-property-finder shape and doesn't capture the actual reference workflow, where the Agent drives multi-step extraction (follow link, read PDF, extract photos) that a single-field assist button can't express.

## Consequences

- Every Agent write path (`save_listing`, `update_context`, `propose_criteria`) must produce data that also renders correctly in a plain form/editor — no chat-only fields.
- `listing.user_confirmed_fields` ([[../../spec/00_infrastructure_setup/00-02-database-schema]]) exists specifically to let manual form edits win over a later Agent write, so the two paths don't fight each other.
- The frontend carries real cost for two coherent surfaces (chat + form) instead of one, but that cost buys the transparency the project exists to fix.
