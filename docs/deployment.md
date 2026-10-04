# Deployment auf einen entfernten Docker-Host

Ziel: Image lokal bauen, als Datei auf den Host kopieren, dort ohne Quellcode starten. Keine Registry nötig.

## 1. Image bauen (Entwicklungsrechner)

```bash
deploy/build-image.sh                        # Ziel linux/amd64 (Standard)
PLATFORM=linux/arm64 deploy/build-image.sh   # ARM-Host (Raspberry Pi, ARM-NAS …)
```

Ergebnis: `deploy/property-finder-image.tar.gz` (~12 MB, gitignored), enthält die Tags
`property-finder:latest` und `property-finder:<git describe>` (z. B. `9a21952-dirty`).

Plattform des Hosts prüfen: `docker version --format '{{.Server.Arch}}'` auf dem Host (`amd64` / `arm64`).
Ein Image der falschen Architektur startet dort nicht (bzw. nur emuliert).

Cross-Build ohne Emulation: `backend/Dockerfile` baut Frontend- und Go-Stage mit `--platform=$BUILDPLATFORM`
und kompiliert per `GOOS=$TARGETOS GOARCH=$TARGETARCH` (CGO aus). Nur die Runtime-Stage (`apk add`) läuft
für die Zielplattform.

## 2. Auf den Host kopieren

```bash
ssh host mkdir -p ~/property-finder
scp deploy/property-finder-image.tar.gz deploy/docker-compose.yml deploy/.env.example host:~/property-finder/
```

## 3. Auf dem Host starten

```bash
cd ~/property-finder
docker load -i property-finder-image.tar.gz
cp .env.example .env          # DB_PASSWORD, APP_PASSWORD und CLAUDE_API_KEY setzen
docker compose up -d
curl localhost:8080/health    # {"status":"ok"}
```

`deploy/docker-compose.yml` unterscheidet sich vom Dev-Compose im Repo-Root:

- `image: property-finder:${PF_IMAGE_TAG:-latest}` statt `build:`.
- Postgres-Port wird nicht veröffentlicht.
- `/data` (PDFs, Bilder) ist das Named Volume `appdata`, kein `./data`-Bind-Mount. Ein neues Named Volume
  übernimmt die Eigentümerschaft von `/data` aus dem Image (uid 10001, User `app`); ein von Docker auf
  Linux automatisch angelegter Bind-Mount-Ordner gehört `root`, und das Backend (läuft nicht als root)
  könnte nicht hineinschreiben.
- Optionale `.env`-Variablen: `PF_HTTP_PORT` (Host-Port, Standard 8080), `PF_IMAGE_TAG` (Standard `latest`).

`CLAUDE_API_KEY` wird nur beim allerersten Start in `app_config` übernommen, danach ist die DB maßgeblich
(siehe AGENTS.md „Config is two-tier“). `DB_PASSWORD` wirkt nur bei der Initialisierung des `pgdata`-Volumes.

## Update

Schritte 1–2 wiederholen, dann auf dem Host:

```bash
docker load -i property-finder-image.tar.gz
docker compose up -d          # erstellt backend mit dem neuen Image neu; Migrationen laufen beim Boot
```

Rollback: `PF_IMAGE_TAG=<alter git-Tag>` in `.env` setzen, `docker compose up -d` — funktioniert, solange
das alte Image noch geladen ist **und** keine neue Migration gelaufen ist (Migrationen sind forward-only).

## Backup

Beide Volumes enthalten Nutzdaten: `<projekt>_pgdata` (Datenbank) und `<projekt>_appdata` (PDFs, Bilder).
Datenbank-Dump: `docker compose exec db pg_dump -U app propertyfinder > backup.sql`.
