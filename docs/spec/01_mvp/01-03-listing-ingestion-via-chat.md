---
type: spec
created: 2026-07-28
title: Listing Ingestion via Chat (URL or PDF)
project: property-finder-obs-style
spec: listing-ingestion-via-chat
phase: 1
order: 3
category: chat
status: ready
updated: 2026-07-28
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - chat
  - ingestion
---

## Overview

The chat-driven counterpart of the reference workflow's core loop: give the Agent a URL or PDF, it reads, follows, extracts, writes, and evaluates. See [[../CONTEXT#Source]] and [[../CONTEXT#Listing Document]].

---

## Flow — URL

1. User pastes a URL (with or without an instruction, e.g. just the link, or "bewerte das bitte")
2. Agent calls `fetch_url` → gets reduced page text, image URLs, outbound links
3. Agent checks Global + Profile Context for an applicable Portal Rule for this domain. If one exists (e.g. "Booli → folge Läs-mer-hos-mäklaren-Link"), it calls `fetch_url` again on the secondary link. If none exists and the page looks materially incomplete (e.g. very short description, a "sign in to see more" wall), the Agent says so in the chat and asks whether to try a specific link it noticed, rather than guessing silently
4. Agent calls `download_image` for photos it judges relevant, tagging each with a category
5. Agent drafts `document_md` (original excerpt, translation, structured fields) and calls `save_listing`
6. Agent calls `evaluate_listing` against the profile's current Criteria
7. Agent's final chat reply summarises the result with the inline Listing card (see [[01-01-chat-interface]])

## Flow — PDF

Same as above from step 4 onward, except step 2–3 are replaced by `read_pdf` — no secondary-link following, since a PDF is the terminal source. If the PDF's images are unclear or the user wants a specific one stored, the user asks and the Agent calls `download_image` against the source page URL if given, or asks the user to also share the original page for image extraction.

---

## Deduplication

Before calling `save_listing` for a URL-sourced import, the Agent checks `list_profile_listings` for an existing Listing with the same `source_url` in this profile. If found, it asks whether to update the existing entry (treated as a Follow-Up Instruction, see [[../CONTEXT#Follow-Up Instruction]]) or proceed anyway (e.g. a genuinely re-listed property). Manual/PDF entries get no automatic dedup — same rationale as [[../../dev-property-finder/spec/01_mvp/01-11-listing-deduplication|dev-property-finder's]] address/coordinate warning, surfaced by the Agent conversationally instead of a UI banner.

---

## What Gets Saved

`save_listing` populates the same structured fields as [[../../dev-property-finder/spec/00_infrastructure_setup/00-02-database-schema#listing|dev-property-finder's listing table]] (title, price, sizes, address, coordinates, seller info where the source page/PDF states it) plus `document_md` and `source_type`/`source_file_path` ([[../00_infrastructure_setup/00-02-database-schema#Changed Tables]]).

---

## Failure Modes

| Situation | Agent behaviour |
|---|---|
| `fetch_url` blocked (anti-bot, 403, CAPTCHA wall) | Reports it in chat, suggests the user upload a PDF of the page instead |
| Page loads but is clearly not a listing (e.g. search results, expired listing) | Says so, does not call `save_listing` |
| Coordinates cannot be determined from the source | Saves the Listing without coordinates; it simply doesn't appear on the Map view until a coordinate is added manually or via a Follow-Up Instruction |
| Criteria not yet defined for this profile | Agent proposes an initial Criteria set (`propose_criteria`) before evaluating, prompting confirmation same as [[../../dev-property-finder/spec/01_mvp/01-02-ai-parsing-criteria|dev-property-finder's initial parse]], adapted to a conversational confirm instead of a form save |

---

## Open Questions / TBD

- Batch ingestion (pasting multiple URLs in one message) — MVP handles one Source per user message; whether to support a list is deferred pending real usage
