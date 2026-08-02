---
type: spec
created: 2026-07-28
title: Search Profile Management
project: property-finder-obs-style
spec: profile-management
phase: 1
order: 8
category: profiles
status: ready
updated: 2026-07-28
tags:
  - property-finder-obs-style
  - spec
  - phase-1
  - profiles
---

## Overview

Search Profiles themselves are managed through a plain form, same rationale as [[../../docs/adr/0001-chat-first-with-form-fallback]]: creating a profile is a one-time, low-frequency, purely structural action (name it) with nothing conversational about it — the Chat only becomes useful once a profile exists to hold its Context and Listings.

---

## Profile List

Landing view of the app: a plain list/grid of existing Search Profiles (name, Listing count, last activity timestamp, top Score in the profile). Clicking a profile opens it on the Chat tab (see [[01-01-chat-interface]]).

---

## Create / Rename / Archive

- **Create**: form with a single required field, `name`. No description field required upfront (unlike [[../../../dev-property-finder/spec/01_mvp/01-01-create-manage-search-profiles|dev-property-finder's]] free-text description driving an immediate parse) — Criteria and Profile Context grow from the Chat and from direct editing ([[01-04-criteria-and-scoring]], [[01-02-context-files]]) once the profile exists, not from a single upfront blob. An optional `description` field is still available for the user's own reference, but it is not itself parsed into Criteria.
- **Rename**: inline edit on `search_profile.name`, no side effects
- **Archive**: soft state via `search_profile.archived_at TIMESTAMPTZ` — hiding the profile from the default Profile List without deleting its Listings/Chat/Context. Archived profiles are shown in a separate "Archiv" section at the bottom of the profile list.
- **Delete**: hard delete, available but requires a confirmation dialog stating it removes all Listings, Chat history, and Context for the profile — no soft-delete/undo, matching dev-property-finder's stated no-soft-delete convention for the rest of the schema

---

## What Lives Where

A Search Profile is the parent of four things, each with its own view/editor, all reachable from the same profile shell:

| Sub-view | Spec |
|---|---|
| Chat | [[01-01-chat-interface]] |
| Profile Context | [[01-02-context-files]] |
| Criteria (list, Weight editing) | [[01-04-criteria-and-scoring]] |
| Listings (List/Ranking/Map + detail form) | [[01-05-ranking-and-list-view]], [[01-06-map-view]], [[01-07-manual-review-and-edit]] |
