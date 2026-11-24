---
type: adr
title: "PDF Sources are read with native Claude document input, with no local extraction pipeline"
project: property-finder-obs-style
status: accepted
created: 2026-07-28
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. It affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#PDF Handling]] and [[../../spec/01_mvp/01-03-listing-ingestion-via-chat#Flow — PDF]].

## Context

The reference Obsidian workflow already has a PDF fallback for pages that block scraping. The user saves the page as a PDF. Then the assistant runs `python3 -c "import fitz"` (pymupdf) to pull text and images out locally. Only after that does it reason over them. This step is manual and brittle. Text extraction depends on layout. Images need a separate extraction pass. It only exists because the assistant in that workflow cannot read a PDF directly.

## Decision

Uploaded PDFs go to Claude as a native `document` content block in the same chat turn. PDFs discovered through secondary or tertiary `fetch_url` links use the same native input path: the Tool returns turn-local document bytes, and the Agent loop appends them after that roundtrip's Tool results. The base64 payload is not stored in `chat_message.tool_result` or resent in later turns. See [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#PDF Handling]]. There is no `internal/pdf` OCR/extraction step and no pymupdf-like dependency. Claude reads text and embedded images directly. `download_image` is only called afterwards. It is called only if the user wants a specific photo copied out and stored with the Listing. The Agent does not need extraction to *understand* the PDF. Only the app needs it, to *save* a chosen image file.

## Alternatives considered

- **Port the pymupdf approach** (extract text and images locally, then give the extracted text to the LLM as before) — rejected. It repeats a workaround that existed only because the Obsidian assistant could not understand PDFs natively. Keeping it would add a dependency and a failure mode (extraction quality, layout edge cases) with no benefit once native input exists.
- **OCR fallback for scanned or image-only PDFs** — not needed as a separate path. Claude's native PDF input already handles image-only pages the same way it handles a JPEG upload.

## Consequences

- `internal/pdf/pdf.go` ([[../../spec/00_infrastructure_setup/00-01-tech-stack#Backend — Go + chi]]) is a thin wrapper (read the file, build the document content block). It is not a parsing library integration.
- PDF size and page-count limits are whatever Claude's API enforces for document input. We set no limit of our own. This needs checking once implementation starts (see Open Questions).
- This decision depends on the provider. If LM Studio or OpenCode are built ([[0006-single-provider-for-agent-mvp]]), their PDF input support (if any) needs separate checking. The `internal/agent.Provider` interface must say whether the provider accepts PDF content blocks. If not, the Agent can fall back to asking the user to paste text.

## Open Questions / TBD

- The current PDF size and page limits of the Claude API, and the cost for large listing PDFs. Check them against the current API docs before implementation. They are not assumed here.
