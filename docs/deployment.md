# Deployment to a remote Docker host

Goal: build the image locally, copy it to the host as a file, and start it there without source code. No registry is needed.

## 1. Build the image (development machine)

```bash
deploy/build-image.sh                        # target linux/amd64 (default)
PLATFORM=linux/arm64 deploy/build-image.sh   # ARM host (Raspberry Pi, ARM NAS …)
```

Result: `deploy/property-finder-image.tar.gz` (~12 MB, gitignored). It has the tags `property-finder:latest` and `property-finder:<git describe>` (for example `9a21952-dirty`).

Check the platform of the host: run `docker version --format '{{.Server.Arch}}'` on the host (`amd64` / `arm64`). An image for the wrong architecture does not start there (or only runs emulated).

Cross-build without emulation: `backend/Dockerfile` builds the frontend and Go stages with `--platform=$BUILDPLATFORM`. It compiles with `GOOS=$TARGETOS GOARCH=$TARGETARCH` (CGO off). Only the runtime stage (`apk add`) runs for the target platform.

## 2. Copy to the host

```bash
ssh host mkdir -p ~/property-finder
scp deploy/property-finder-image.tar.gz deploy/docker-compose.yml deploy/.env.example host:~/property-finder/
```

## 3. Start on the host

```bash
cd ~/property-finder
docker load -i property-finder-image.tar.gz
cp .env.example .env          # set DB_PASSWORD, APP_PASSWORD and CLAUDE_API_KEY
docker compose up -d
curl localhost:8080/health    # {"status":"ok"}
```

`deploy/docker-compose.yml` differs from the dev compose file in the repo root:

- `image: property-finder:${PF_IMAGE_TAG:-latest}` instead of `build:`.
- The Postgres port is not published.
- `/data` (PDFs, images) is the named volume `appdata`, not a `./data` bind mount. A new named volume takes the ownership of `/data` from the image (uid 10001, user `app`). A bind-mount folder that Docker creates automatically on Linux belongs to `root`. The backend does not run as root, so it could not write into it.
- Optional `.env` variables: `PF_HTTP_PORT` (host port, default 8080), `PF_IMAGE_TAG` (default `latest`).

`CLAUDE_API_KEY` is copied into `app_config` only at the very first start. After that, the DB is the source of truth (see AGENTS.md "Config is two-tier"). `DB_PASSWORD` only has an effect when the `pgdata` volume is first initialized.

## Update

Repeat steps 1–2. Then run this on the host:

```bash
docker load -i property-finder-image.tar.gz
docker compose up -d          # recreates backend with the new image; migrations run at boot
```

Rollback: set `PF_IMAGE_TAG=<old git tag>` in `.env`, then run `docker compose up -d`. This works as long as the old image is still loaded **and** no new migration has run (migrations are forward-only).

## Backup

Both volumes hold user data: `<project>_pgdata` (database) and `<project>_appdata` (PDFs, images).
Database dump: `docker compose exec db pg_dump -U app propertyfinder > backup.sql`.
