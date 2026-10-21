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

Accepted. This extends [[0005-pdf-native-document-input]]. It affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#PDF Handling]].

## Context

ADR 0005 lets a user paste a PDF export of a listing page when the page blocks scraping. On a phone there
is often no way to make a PDF at all. There is only a screenshot (JPG in most Android browsers, PNG on
iOS). The user still needs a way to get that listing into a chat turn without going to a desktop.

## Decision

Chat-uploaded JPG/PNG files are sent to Claude as a native `image` content block in the same turn. This
mirrors the PDF path of ADR 0005 exactly:

- `POST /api/upload/image` (`UploadHandler.Image`, `UploadService.SaveImage`) checks the JPEG/PNG magic
  bytes. It writes the file to `{DataDir}/uploads/image/{uuid}.{jpg,png}`. The raw-byte cap is 3.75 MB.
  This keeps the base64-encoded payload within Claude's 5 MB image limit.
- The returned path travels in the existing `chat_message.attachment_path` column. There is no new column
  and no new attachment type or table. The `attachedImagePath` helper in `AgentService.runTurn` (it mirrors
  `attachedPDFPath`) decides between PDF, image or neither. It uses only the extension of the stored path.
- `internal/pdf` is renamed to `internal/attachment`. `ReadBase64` and `Delete` already worked for any
  file type. Only their doc comments said "PDF". Images now use them too.
- `agent.ContentBlock` gets `ImageData`/`ImageMediaType` fields and a `ContentBlockImage` type. It also
  gets a new capability gate, `Provider.SupportsImageInput()`. This is separate from
  `SupportsDocumentInput()`, because a future provider could support one content type but not the other
  (same reasoning as the Consequences section of ADR 0005).
- Like a PDF, the uploaded image is deleted from disk when its ingestion turn ends. It is never sent again
  in a later turn.

## Alternatives considered

- **Reuse `ContentBlockDocument` for images too** (Claude's document block also accepts an image
  source) — rejected. It mixes two different Anthropic block types and their media-type handling under
  one Go type. A dedicated `ContentBlockImage` keeps the switch in `formatMessages` and the capability
  gate honest about what each provider really received.
- **A shared `/api/upload/attachment` endpoint with server-side type sniffing** (one route for PDF and
  image) — rejected. With `UploadHandler.PDF` and `UploadHandler.Image` kept apart, each has its own
  size limit and simple magic-byte check, and each is easy to test alone. The client-side `accept`
  filters of the two upload buttons already tell the server which type is coming.

## Consequences

- Same as ADR 0005: this is provider-specific. A future non-Claude `agent.Provider` must implement
  `SupportsImageInput()` honestly. `agent.NewUnconfiguredProvider()` returns `false`.
- No DB migration is needed. The existing TEXT column `attachment_path` and the extension-based dispatch
  already cover a second file type for free.
