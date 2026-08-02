---
type: adr
title: "PDF Sources are read via native Claude document input, no local extraction pipeline"
project: property-finder-obs-style
status: accepted
created: 2026-07-28
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#PDF Handling]], [[../../spec/01_mvp/01-03-listing-ingestion-via-chat#Flow — PDF]].

## Context

The reference Obsidian workflow already has a PDF fallback for pages that block scraping: the user saves the page as PDF, and the assistant runs `python3 -c "import fitz"` (pymupdf) to pull text and images out locally before reasoning over them ([[../../../immobilienplan-schweden-portugal-oesterreich/schweden/00_readme#2. Bilder auswerten]]). That's a manual, brittle step — layout-dependent text extraction, a separate image-extraction pass, and it only exists because the assistant driving that workflow can't read a PDF directly.

## Decision

Uploaded PDFs are sent to Claude as a native `document` content block in the same chat turn — see [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#PDF Handling]]. No `internal/pdf` OCR/extraction step, no pymupdf-equivalent dependency. Claude reads text and embedded images directly. `download_image` is only called afterwards, and only if the user wants a specific photo copied out and stored against the Listing — extraction is not a prerequisite for the Agent to *understand* the PDF, only for the app to *persist* a chosen image file.

## Alternatives considered

- **Port the pymupdf approach** (extract text + images locally, feed extracted text to the LLM as before) — rejected: reproduces a workaround that existed only because the Obsidian assistant lacked native PDF understanding; carrying it forward would add a dependency and a failure mode (extraction quality, layout edge cases) for no benefit once native input is available.
- **OCR fallback for scanned/image-only PDFs** — not needed as a separate path: Claude's native PDF input already handles image-only pages the same way it handles a JPEG upload.

## Consequences

- `internal/pdf/pdf.go` ([[../../spec/00_infrastructure_setup/00-01-tech-stack#Backend — Go + chi]]) is a thin wrapper (read file, build the document content block) rather than a parsing library integration
- PDF size/page-count limits are whatever Claude's API enforces for document input, not a self-imposed limit — needs verification once implementation starts (see Open Questions)
- This decision is provider-specific: if/when LM Studio or OpenCode are implemented ([[0006-single-provider-for-agent-mvp]]), their PDF input capability (if any) needs separate verification — the `internal/agent.Provider` interface must expose "does this provider accept PDF content blocks" so the Agent can fall back to asking the user to paste text if not

## Open Questions / TBD

- Claude API's current PDF size/page limits and cost implications for large listing PDFs — verify against current API docs before implementation, not assumed here
