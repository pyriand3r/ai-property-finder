---
type: adr
title: "Chat is the main interface; a plain form is a required fallback"
project: property-finder-obs-style
status: accepted
created: 2026-07-28
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. This is a foundation decision. It affects [[../../spec/01_mvp/01-01-chat-interface]] and [[../../spec/01_mvp/01-07-manual-review-and-edit]].

## Context

The old spec of the predecessor project got the interaction model wrong. This project exists to fix that. Form-driven CRUD does not match how the workflow runs in practice (in the reference Obsidian workflow). The obvious fix is "make the Agent chat the only interface." That is a trap, because a chat-only app is not auditable. Say the Agent silently reads a price wrong or picks the wrong coordinates. If the only way to see or fix that is another chat turn, the user must *trust* the Agent completely. That is the opposite of "der User muss alles gut nachvollziehen und auch manuell anpassen können" from the original brief (means: the user must be able to follow everything easily and also change it by hand).

## Decision

Chat is the main and fastest way to get work done. Examples are ingesting a Source, asking for comparisons, and giving Follow-Up Instructions. But every structured field the Agent writes is also visible in a plain form. The user can edit it there directly (the Listing detail view, [[../../spec/01_mvp/01-07-manual-review-and-edit]]). No chat step is needed to reach or use it. The form is not a secondary or advanced view. The app depends on it.

## Alternatives considered

- **Chat-only, no form** — rejected. There is no fallback when the Agent gets something wrong or is unreachable (LLM outage). There is also no fast way to fix ten small errors across many Listings without ten chat turns.
- **Form-only with an AI "assist" button per field** — rejected. This is closer to the shape of the predecessor project. It does not fit the real reference workflow. There the Agent drives multi-step extraction (follow a link, read a PDF, extract photos). A single-field assist button cannot express that.

## Consequences

- Every Agent write path (`save_listing`, `update_context`, `propose_criteria`) must produce data that also shows correctly in a plain form or editor. No field may exist only in chat.
- `listing.user_confirmed_fields` ([[../../spec/00_infrastructure_setup/00-02-database-schema]]) exists so that manual form edits win over a later Agent write. This stops the two paths from fighting each other.
- The frontend pays a real cost for two consistent surfaces (chat + form) instead of one. That cost buys the transparency this project exists to provide.
