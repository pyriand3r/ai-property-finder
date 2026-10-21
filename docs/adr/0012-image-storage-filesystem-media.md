---
type: adr
title: "Image storage: files under /data, served via /media, title_image_id on listing"
project: property-finder-obs-style
status: accepted
created: 2026-08-01
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. It affects [[../../spec/00_infrastructure_setup/00-01-tech-stack]], [[../../spec/00_infrastructure_setup/00-02-database-schema]] and [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration]].

## Context

The `download_image` Tool gets images from listing pages and saves them locally so the frontend can show them. Three things were not yet defined: where the files live on disk, how the frontend loads them, and which image is the listing's preview thumbnail in list and map views.

We quickly dropped a database-only approach (base64 or BYTEA). Images are large, PostgreSQL is not a file server, and the app already needs a file-serving layer for PDF uploads. The simplest way is a directory mounted as a volume, with a static file server.

## Decision

### Filesystem layout

```
/data/images/listings/{listing_id}/{uuid}.{ext}
```

- `/data` is a Docker volume mount (`- ./data:/data` or a named volume).
- The UUID comes from `gen_random_uuid()`. `download_image` assigns it before the download.
- The extension is kept from the source URL (`.jpg`, `.png`, `.webp`).

### Serving

The Go backend serves `/media/` as a static file handler that points at `/data/`. The frontend refers to images like this:

```
/media/images/listings/{listing_id}/{uuid}.jpg
```

There is no separate nginx container, no CDN and no signed URLs. The app has one user and runs on a local network. A plain `http.FileServer` is enough.

### Database

The `listing` table gets one new column:

| Column | Type | Constraints | Description |
|---|---|---|---|
| `title_image_id` | UUID | FK → listing_image, NULLABLE, ON DELETE SET NULL | The listing_image row used as the preview thumbnail in list and map views. The Agent sets it on the first download. The user can change it in the UI. |

The `listing_image` table gets one column (already specified, now confirmed):

| Column | Type | Constraints | Description |
|---|---|---|---|
| `category` | TEXT | NULLABLE | Set by the Agent: `exterior` \| `interior` \| `floor_plan` \| `outbuilding` \| `other` |

It also gets one new column for the file path:

| Column | Type | Constraints | Description |
|---|---|---|---|
| `file_path` | TEXT | NOT NULL | Path relative to `/data/`: `images/listings/{listing_id}/{uuid}.jpg` |

### Title image selection

There are two ways, and no others:

1. **The Agent sets the first one.** During `download_image`, the Agent picks a title image. This is usually the first exterior photo, or the best-quality image it can find. It calls `save_listing` with `title_image_id` set.
2. **The user changes it in the UI.** The listing detail view shows a thumbnail gallery. Each image has a star icon. A click on the star sets `listing.title_image_id` to that image's UUID. No chat command is needed.

The Agent never changes the title image after the first one is set. If the user chose one by hand, that choice is final. If the title image is deleted, `ON DELETE SET NULL` clears the column. The next `download_image` call, or the user, can then set a new one.

### `download_image` Tool behaviour

The Tool gets an image URL and a listing ID. It does these steps:

1. Downloads the image with an HTTP GET.
2. Finds the original extension from the Content-Type or the URL path.
3. Creates a UUID file name and writes the file to `/data/images/listings/{listing_id}/{uuid}.{ext}`.
4. Inserts a `listing_image` row (`listing_id`, `file_path`, and `category` from the Agent's input).
5. If the listing has no `title_image_id` yet, sets it to this row's ID.
6. Returns `{"status": "ok", "image_id": "<uuid>", "path": "..."}`.

### Cleanup

When a listing is deleted, the delete handler does this:

```go
os.RemoveAll(filepath.Join("/data", "images", "listings", listingID.String()))
// ... then DELETE FROM listing WHERE id = ...
```

The `listing_image` rows are deleted through the FK cascade. The files are always removed. There are no orphaned files and no cron job.

## Alternatives considered

- **Store images in PostgreSQL as BYTEA**: rejected. It makes the database large, slows backups, and PostgreSQL is not built to serve large binary blobs to a frontend.
- **A separate image service (MinIO, etc.)**: rejected. It is too much for an app with one user on a local network. A directory plus Go's `http.FileServer` is 5 lines of code.
- **Pick the title image with a chat command** ("mach Bild 3 zum Titelbild", meaning: make image 3 the title image): rejected. It makes the Agent more complex for something the user can do with one click in the gallery UI. The Agent sets it once. The user changes it in the UI.
- **A title image rule in the Profile Context** ("bei schwedischen Objekten Grundriss als Titelbild", meaning: for Swedish properties, use the floor plan as the title image): rejected. It is over-engineered. The user can change it in 2 seconds in the UI.

## Consequences

- Docker Compose needs a volume mount: `- ./data:/data` (or a named volume).
- The Go backend serves `/media/` from `http.Dir("/data")`. This is one route in the chi router.
- The `download_image` tool is about 40 lines: download, find the extension, rename to a UUID, insert into the DB, and optionally update `title_image_id`.
- The listing delete path gets an `os.RemoveAll` call before the DB cascade. This is two lines and needs no new package.
- `listing_image.file_path` is a relative path, not an absolute one. The frontend builds the full URL from the `/media/` base.
