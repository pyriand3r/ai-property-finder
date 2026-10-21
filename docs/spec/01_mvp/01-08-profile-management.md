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

You manage Search Profiles themselves with a plain form. The reason is the same as in [[../../docs/adr/0001-chat-first-with-form-fallback]]. Creating a profile happens once and rarely. It is a purely structural action (give it a name). Nothing about it is conversational. The Chat is only useful once a profile exists to hold its Context and Listings.

---

## Profile List

This is the landing view of the app. It is a plain list or grid of existing Search Profiles. Each entry shows the name, the Listing count, the time of last activity, and the top Score in the profile. A click on a profile opens it on the Chat tab (see [[01-01-chat-interface]]).

---

## Create / Rename / Archive

- **Create**: A form with one required field, `name`. No description is required up front. This differs from the free-text description of the predecessor project, which was parsed right away. Here, Criteria and Profile Context grow from the Chat and from direct editing ([[01-04-criteria-and-scoring]], [[01-02-context-files]]) after the profile exists. They do not come from one big text up front. An optional `description` field is still available for the user's own reference. It is not parsed into Criteria.
- **Rename**: Inline edit of `search_profile.name`. It has no side effects.
- **Archive**: A soft state through `search_profile.archived_at TIMESTAMPTZ`. It hides the profile from the default Profile List. It does not delete its Listings, Chat, or Context. Archived profiles show in a separate "Archiv" section at the bottom of the profile list.
- **Delete**: A hard delete. It is available, but it needs a confirmation dialog. The dialog says that it removes all Listings, Chat history, and Context of the profile. There is no soft-delete and no undo. This matches the no-soft-delete rule of the predecessor project for the rest of the schema.

---

## What Lives Where

A Search Profile is the parent of four things. Each has its own view or editor. You reach all of them from the same profile shell:

| Sub-view | Spec |
|---|---|
| Chat | [[01-01-chat-interface]] |
| Profile Context | [[01-02-context-files]] |
| Criteria (list, Weight editing) | [[01-04-criteria-and-scoring]] |
| Listings (List/Ranking/Map + detail form) | [[01-05-ranking-and-list-view]], [[01-06-map-view]], [[01-07-manual-review-and-edit]] |
