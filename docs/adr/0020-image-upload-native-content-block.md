---
type: adr
title: "JPG/PNG screenshots are read via native Claude image input, same pattern as PDF"
project: property-finder-obs-style
status: accepted
created: 2026-10-04
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — extends [[0005-pdf-native-document-input]]. Affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#PDF Handling]].

## Context

ADR 0005 lets a user paste a PDF export of a listing page when the page blocks scraping. On a phone
there is often no way to produce a PDF at all — only a screenshot (JPG on most Android browsers, PNG on
iOS). The user still needs a way to get that listing into a chat turn without a desktop detour.

## Decision

Chat-uploaded JPG/PNG files are sent to Claude as a native `image` content block in the same turn,
mirroring ADR 0005's PDF path exactly:

- `POST /api/upload/image` (`UploadHandler.Image`, `UploadService.SaveImage`) validates the JPEG/PNG
  magic bytes, writes to `{DataDir}/uploads/image/{uuid}.{jpg,png}`, 3.75 MB raw-byte cap (keeps the
  base64-encoded payload within Claude's 5 MB image limit).
- The returned path rides on the existing `chat_message.attachment_path` column — no new column, no new
  attachment type/table. `AgentService.runTurn`'s `attachedImagePath` helper (mirrors `attachedPDFPath`)
  decides PDF vs. image vs. neither purely from the stored path's extension.
- `internal/pdf` is renamed to `internal/attachment` (`ReadBase64`/`Delete` were already file-type
  agnostic — only their doc comments said "PDF"), now also used for images.
- `agent.ContentBlock` gains `ImageData`/`ImageMediaType` fields and a `ContentBlockImage` type,
  alongside a new `Provider.SupportsImageInput()` capability gate — separate from
  `SupportsDocumentInput()`, since a future provider could support one content type but not the other
  (same reasoning as ADR 0005's Consequences section).
- Like a PDF, the uploaded image is deleted from disk once its ingestion turn ends and is never resent
  on a later turn.

## Alternatives considered

- **Reuse `ContentBlockDocument` for images too** (Claude's document block also accepts an image
  source) — rejected: conflates two different Anthropic block types and media-type handling under one
  Go type; a dedicated `ContentBlockImage` keeps `formatMessages`'s switch and the capability gate
  honest about what each provider actually received.
- **A shared `/api/upload/attachment` endpoint with server-side type sniffing** (one route for PDF and
  image) — rejected: `UploadHandler.PDF`/`UploadHandler.Image` keep their own size limits and magic-byte
  validation simple and independently testable; the two upload buttons' client-side `accept` filters
  already tell the server which one is coming.

## Consequences

- Same as ADR 0005's: this is provider-specific. A future non-Claude `agent.Provider` must implement
  `SupportsImageInput()` honestly — `agent.NewUnconfiguredProvider()` returns `false`.
- No DB migration — `attachment_path`'s existing TEXT column and extension-based dispatch already cover
  a second file type for free.
