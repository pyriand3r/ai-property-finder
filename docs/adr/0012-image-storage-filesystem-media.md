---
type: adr
title: "Image storage: filesystem under /data, served via /media, title_image_id on listing"
project: property-finder-obs-style
status: accepted
created: 2026-08-01
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — affects [[../../spec/00_infrastructure_setup/00-01-tech-stack]], [[../../spec/00_infrastructure_setup/00-02-database-schema]], [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]].

## Context

The `download_image` Tool fetches images from listing pages and stores them locally for the frontend to display. Three things were unspecified: where the files live on disk, how the frontend loads them, and which image acts as the listing's preview thumbnail in list/map views.

A database-only approach (base64 or BYTEA) was quickly discarded — images are large, PostgreSQL is not a file server, and the app already needs a file-serving layer for PDF uploads. The simplest path is a volume-mounted directory with a static file server.

## Decision

### Filesystem layout

```
/data/images/listings/{listing_id}/{uuid}.{ext}
```

- `/data` — Docker volume mount (`- ./data:/data` or named volume)
- UUID — `gen_random_uuid()` assigned by `download_image` before download
- Extension — preserved from the source URL (`.jpg`, `.png`, `.webp`)

### Serving

The Go backend serves `/media/` as a static file handler pointing at `/data/`. The frontend references images as:

```
/media/images/listings/{listing_id}/{uuid}.jpg
```

No separate nginx container, no CDN, no signed URLs. The app is single-user, local-network — a plain `http.FileServer` is sufficient.

### Database

`listing` table gains one column:

| Column | Type | Constraints | Description |
|---|---|---|---|
| `title_image_id` | UUID | FK → listing_image, NULLABLE, ON DELETE SET NULL | The listing_image row used as preview thumbnail in list/map views. Set by the Agent on first download, changed by the user in the UI. |

`listing_image` table gains one column (already specified, confirmed):

| Column | Type | Constraints | Description |
|---|---|---|---|
| `category` | TEXT | NULLABLE | Agent-assigned: `exterior` \| `interior` \| `floor_plan` \| `outbuilding` \| `other` |

Plus one new column for the file path:

| Column | Type | Constraints | Description |
|---|---|---|---|
| `file_path` | TEXT | NOT NULL | Relative path from `/data/`: `images/listings/{listing_id}/{uuid}.jpg` |

### Title image selection

Two paths, no others:

1. **Agent sets initial** — during `download_image`, the Agent picks a title image (typically the first exterior photo, or the best-quality image it can identify). Calls `save_listing` with `title_image_id` set.
2. **User changes in UI** — the listing detail view shows a thumbnail gallery. Each image has a star icon; clicking it sets `listing.title_image_id` to that image's UUID. No chat command needed.

The Agent never changes the title image after initial creation — if the user set one manually, that's final. If the title image is deleted, `ON DELETE SET NULL` clears the column; the next `download_image` call or the user can set a new one.

### `download_image` Tool behaviour

The Tool receives an image URL and a listing ID. It:

1. Downloads the image via HTTP GET
2. Detects the original extension from Content-Type or URL path
3. Generates a UUID filename, writes to `/data/images/listings/{listing_id}/{uuid}.{ext}`
4. Inserts a `listing_image` row (`listing_id`, `file_path`, `category` from Agent input)
5. If the listing has no `title_image_id` set yet, sets it to this row's ID
6. Returns `{"status": "ok", "image_id": "<uuid>", "path": "..."}`

### Cleanup

When a listing is deleted, the delete handler:

```go
os.RemoveAll(filepath.Join("/data", "images", "listings", listingID.String()))
// ... then DELETE FROM listing WHERE id = ...
```

`listing_image` rows cascade via FK. Files are gone deterministically. No orphaned files, no cron job.

## Alternatives considered

- **Store images in PostgreSQL as BYTEA** — rejected: bloats the DB, slows backups, PostgreSQL is not optimized for serving large binary blobs to a frontend.
- **Separate image service (MinIO, etc.)** — rejected: overkill for a single-user, local-network app. A directory + Go's `http.FileServer` is 5 lines of code.
- **Title image selected by chat command** ("mach Bild 3 zum Titelbild") — rejected: adds complexity to the Agent for something the user can do with one click in the gallery UI. The Agent sets it once; the user changes it in the UI.
- **Title image rule in Profile Context** ("bei schwedischen Objekten Grundriss als Titelbild") — rejected: over-engineered. The user can change it in 2 seconds in the UI.

## Consequences

- Docker Compose needs a volume mount: `- ./data:/data` (or a named volume)
- Go backend serves `/media/` → `http.Dir("/data")`, one route in the chi router
- `download_image` tool implementation is ~40 lines: download, detect extension, UUID-rename, DB insert, optional `title_image_id` update
- Listing delete path gets a `os.RemoveAll` call before the DB cascade — two lines, no new package
- `listing_image.file_path` is a relative path, not absolute — the frontend constructs the full URL from the `/media/` base
