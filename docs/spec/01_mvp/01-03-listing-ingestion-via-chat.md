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

This is the chat version of the core loop of the reference Obsidian workflow. The user gives the Agent a URL or PDF. The Agent reads it, follows links, extracts data, writes the Listing, and evaluates it. See [[../CONTEXT#Source]] and [[../CONTEXT#Listing Document]].

---

## Flow — URL

1. The user pastes a URL. The user may add an instruction, or send only the link. An example instruction is "bewerte das bitte" (means: "please rate this").
2. The Agent calls `fetch_url`. It gets reduced page text, image URLs, and outbound links.
3. The Agent follows relevant secondary and tertiary links with `fetch_url`, for example portal → broker page → floor-plan PDF. A matching Portal Rule in Global or Profile Context identifies important links (for example, "Booli → folge Läs-mer-hos-mäklaren-Link", means: "Booli → follow the 'Läs mer hos mäklaren' link"). The Agent can also follow a clearly labelled link to more listing details, an exposé, or a floor plan without a rule. A linked PDF arrives as a native document in the same ingestion turn. If the page looks incomplete and no clearly relevant link exists, the Agent says so and asks whether to try a specific link it noticed.
4. The Agent calls `download_image` for the photos it thinks are relevant. It tags each photo with a category.
5. The Agent drafts `document_md` (original excerpt, translation, structured fields). It calls `save_listing`.
6. The Agent calls `evaluate_listing` against the profile's current Criteria.
7. The Agent's final chat reply sums up the result. It includes the inline Listing card (see [[01-01-chat-interface]]).

## Flow — PDF

The flow is the same as above from step 4 on. Steps 2–3 are replaced by `read_pdf`. The Agent does not follow second links, because a PDF is the end source. Sometimes the PDF images are unclear, or the user wants a specific image stored. Then the user asks the Agent. If a source page URL is given, the Agent calls `download_image` against it. If not, the Agent asks the user to also share the original page, so it can extract images.

---

## Deduplication

Before the Agent calls `save_listing` for a URL import, it checks `list_profile_listings`. It looks for an existing Listing in this profile with the same `source_url`. If it finds one, it asks the user what to do. The user can update the existing entry. This is treated as a Follow-Up Instruction (see [[../CONTEXT#Follow-Up Instruction]]). Or the user can import anyway, for example for a property that was really listed again. Manual and PDF entries get no automatic dedup. The reason is the same as for the address/coordinate warning in the predecessor project. Here the Agent gives the warning in the conversation, not in a UI banner.

---

## What Gets Saved

`save_listing` fills the same structured fields as the listing table of the predecessor project. These are title, price, sizes, address, coordinates, and seller info, when the source page or PDF states them. It also fills `document_md` and `source_type`/`source_file_path` ([[../00_infrastructure_setup/00-02-database-schema#Changed Tables]]).

---

## Failure Modes

| Situation | Agent behaviour |
|---|---|
| `fetch_url` is blocked (anti-bot, 403, CAPTCHA wall) | Reports it in chat. Suggests that the user uploads a PDF of the page instead. |
| Page loads but is clearly not a listing (for example, search results or an expired listing) | Says so. Does not call `save_listing`. |
| Coordinates cannot be found in the source | Saves the Listing without coordinates. It does not appear on the Map view until someone adds coordinates by hand or through a Follow-Up Instruction. |
| Criteria are not yet defined for this profile | The Agent proposes a first Criteria set (`propose_criteria`) before it evaluates. It asks for confirmation, like the initial parse in the predecessor project. Here the confirmation happens in the conversation, not by saving a form. |

---

## Open Questions / TBD

- Batch ingestion (pasting several URLs in one message): the MVP handles one Source per user message. Support for a list waits until we see real usage.
