---
type: guide
title: Betriebsanleitung (Server-Betrieb)
project: property-finder-obs-style
created: 2026-10-04
tags:
  - property-finder-obs-style
  - operations
  - mcp
---

# Betriebsanleitung

Praktische Anleitung zum Betreiben des Backends (Go, embedded Vue-SPA, Postgres 18) und
zur Nutzung des MCP-Servers. Für das Bauen/Kopieren eines Images auf einen entfernten
Docker-Host ohne Registry siehe [deployment.md](deployment.md) — dieses Dokument
behandelt den laufenden Betrieb (beide Varianten nutzen dieselbe Konfiguration und
dieselben Backup/MCP-Mechanismen).

## 1. Varianten

| Variante | Compose-Datei | Wann |
|---|---|---|
| Lokale Entwicklung/Test | `docker-compose.yml` (Repo-Wurzel) | `build:` aus Quellcode, Postgres-Port veröffentlicht, `./data` Bind-Mount |
| Produktiv auf entferntem Host | `deploy/docker-compose.yml` | `image:` aus vorgebautem Tarball, kein Postgres-Port nach außen, `/data` als Named Volume — siehe [deployment.md](deployment.md) |

Beide Varianten starten/stoppen identisch (`docker compose up -d` / `down`); die
folgenden Abschnitte gelten für beide, sofern nicht anders vermerkt.

## 2. Start / Stop / Status

```bash
docker compose up -d --build       # lokal: baut und startet
docker compose up -d               # Produktiv-Host: startet mit geladenem Image
docker compose ps
docker compose logs -f backend
docker compose down                # stoppt, Volumes bleiben erhalten
curl localhost:8080/health         # {"status":"ok"} — einziger unauthentifizierter Endpoint
```

Der Healthcheck (`GET /health`) ist absichtlich ohne Login erreichbar, da beide
Compose-Dateien ihn per `wget` ohne Auth abfragen.

## 3. Konfiguration

Zwei Konfigurations-Ebenen (`backend/internal/config/config.go`):

### Tier 1 — Environment (`.env`, nur beim Start gelesen)

| Variable | Pflicht | Default | Bedeutung |
|---|---|---|---|
| `DATABASE_URL` | ja | – | Postgres-Connection-String |
| `APP_PASSWORD` | ja | – | Login-Passwort für UI + API (ADR 0018) **und** Bearer-Token für `/mcp` (ADR 0019) |
| `DB_PASSWORD` | ja (nur Docker Compose) | – | Postgres-Passwort; wirkt nur bei der **Erstinitialisierung** des `pgdata`-Volumes, danach ignoriert |
| `CLAUDE_API_KEY` | für den Agent nötig | `""` | wird nur beim allerersten Boot einmalig in `app_config` geschrieben (s. Tier 2); danach ist die DB maßgeblich |
| `COOKIE_SECURE` | nein | `true` | `Secure`-Flag der Session-Cookie; `false` nur für lokale Entwicklung ohne vorgeschalteten TLS-Proxy |
| `HTTP_LISTEN_ADDR` | nein | `:8080` | Bind-Adresse des HTTP-Servers |
| `DATA_DIR` | nein | `/data` | Wurzelverzeichnis für hochgeladene PDFs und Listing-Bilder |

Ein fehlender `DATABASE_URL` oder `APP_PASSWORD` lässt den Start fehlschlagen (fail-fast,
kein stiller Teilbetrieb). Ohne `CLAUDE_API_KEY` startet das Backend trotzdem — der Agent
fällt dann auf `agent.NewUnconfiguredProvider()` zurück und lehnt jede Chat-Anfrage mit
einer entsprechenden Fehlermeldung ab; REST-API, Formulare und MCP-Tools, die keinen
LLM-Call brauchen (`fetch_url`, `download_image`, `geocode`, `list_profile_listings`,
`compute_ranking`, `list_profiles`), funktionieren weiterhin.

### Tier 2 — `app_config`-Tabelle (DB-autoritativ nach dem ersten Boot)

LLM-Einstellungen (`provider`, `base_url`, `model`, `timeout_seconds`, `max_retries`,
`max_tool_calls_per_turn`, `max_llm_roundtrips`, `reasoning_language`, `api_key`) liegen
als Key-Value-Zeilen (`key`, `category`, `value`) in `app_config`. `GET /api/config`
zeigt den aufgelösten Stand **nur lesend** (der API-Key wird nie im Klartext
zurückgegeben, nur `api_key_configured: true/false`) — es gibt bewusst keinen
PUT-Endpoint und keine Einstellungsseite im Frontend. Eine Änderung (z. B. Modellwechsel)
geht direkt über SQL:

```bash
docker compose exec db psql -U app -d propertyfinder -c \
  "INSERT INTO app_config (key, category, value) VALUES ('llm.model', 'llm', 'claude-sonnet-5')
   ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;"
```

Ein Neustart des Backends ist dafür **nicht** nötig — `LoadAll` liest `app_config` pro
Request-Pfad, der die Konfiguration braucht (nicht einmalig beim Boot gecacht).

### Passwort ändern

`APP_PASSWORD` in `.env` ändern, dann `docker compose up -d` (nur den `backend`-Service
neu erzeugen). Da der JWT-Signing-Key von `APP_PASSWORD` abgeleitet wird, invalidiert das
**sofort** jede bestehende Browser-Session **und** jeden MCP-Bearer-Token — gewolltes
Verhalten bei einem Single-User-System (ADR 0018), kein Bug.

## 4. Backup

Zwei Volumes enthalten Nutzdaten:

```bash
# Datenbank
docker compose exec db pg_dump -U app propertyfinder > backup.sql

# PDFs/Bilder — lokal: einfach das Bind-Mount-Verzeichnis sichern
tar czf data-backup.tar.gz ./data
# Produktiv-Host (Named Volume appdata statt Bind-Mount):
docker run --rm -v <projekt>_appdata:/data -v "$PWD":/backup alpine \
  tar czf /backup/data-backup.tar.gz -C / data
```

Restore: Datenbank mit `psql -U app -d propertyfinder < backup.sql` einspielen (auf einer
frisch migrierten, leeren DB), Daten-Tarball in das jeweilige Volume zurück entpacken.

## 5. Migrationen

Laufen automatisch und fail-fast beim Boot vor dem HTTP-Listener
(`backend/cmd/server/main.go`), forward-only (`golang-migrate/v4`,
`backend/internal/db/migrations/`). Es gibt kein separates Migrations-CLI — ein Rollback
auf eine alte Image-Version funktioniert nur, solange danach keine neue Migration
gelaufen ist.

## 6. Troubleshooting

| Symptom | Ursache | Fix |
|---|---|---|
| Login liefert 204, danach sofortiger Redirect zu `/login` | `COOKIE_SECURE=true` (Default) bei einem Deployment ohne TLS-Proxy davor | `COOKIE_SECURE=false` setzen (nur für Setups ohne vorgeschalteten HTTPS-Proxy) |
| Backend-Container `unhealthy`/restartet dauernd | Migration schlägt fehl oder `DATABASE_URL`/`APP_PASSWORD` fehlt | `docker compose logs backend` — Fehler steht in den ersten Zeilen (fail-fast vor dem Listener) |
| Chat antwortet mit Fehler, aber REST/Formulare funktionieren | `CLAUDE_API_KEY` nie gesetzt → `agent.NewUnconfiguredProvider()` | `CLAUDE_API_KEY` in `.env` setzen und Backend neu erzeugen (einmaliger Seed) |
| `POST /mcp` liefert die SPA (`index.html`) statt JSON-RPC | läuft ein Image **vor** ADR 0019 (kein `/mcp`-Route vorhanden) | Image neu bauen/laden (`docker compose up -d --build` bzw. aktuelles `property-finder-image.tar.gz` laden) |
| `POST /mcp` liefert `401` | fehlender/falscher `Authorization: Bearer <APP_PASSWORD>`-Header | Header prüfen; `APP_PASSWORD`-Rotation invalidiert alte Tokens sofort |

## 7. MCP-Server-Nutzung

Der komplette interne Agent-Tool-Katalog ist zusätzlich extern per MCP (Streamable HTTP
Transport, `github.com/modelcontextprotocol/go-sdk`) erreichbar — für externe
MCP-fähige Clients wie Claude Code, Claude Desktop und opencode, ohne eine echte
Chat-Session anzulegen (ADR 0019).

### Endpoint & Auth

- `POST /mcp` — Streamable HTTP, JSON-RPC 2.0, Antworten als einzelnes JSON-Objekt
  (`JSONResponse: true`, keine SSE-Streams; jedes Tool liefert ein schnelles, einmaliges
  Ergebnis).
- Auth: `Authorization: Bearer <APP_PASSWORD>` — derselbe Wert wie das UI-Login-Passwort,
  kein zweites Secret (`RequireBearer`-Middleware, `backend/internal/api/auth.go`).
- Der Handler ist **stateful**: die erste Antwort (`initialize`) liefert einen
  `Mcp-Session-Id`-Response-Header; dieser muss bei allen folgenden Requests derselben
  Sitzung als Header mitgeschickt werden.

### Tool-Katalog

`list_profiles` ist der Einstiegspunkt — er liefert die `profile_id`, die jedes andere
Tool als Pflichtfeld erwartet (es gibt sonst keinen Weg, außerhalb einer Chat-Session an
eine Profil-UUID zu kommen).

| Tool | Zweck | Wichtige Parameter |
|---|---|---|
| `list_profiles` | Alle Search Profiles mit UUID auflisten | – |
| `fetch_url` | Webseite per GET laden, lesbaren Haupttext + Bild-URLs + Links zurückgeben | `profile_id`, `url` |
| `save_listing` | Inserat anlegen/aktualisieren | `profile_id`, `listing_id` (optional = Update), `fields.*` (title, price, address, document_md, seller_*, …), `force_overwrite[]` |
| `download_image` | Bild von URL laden, beim Inserat speichern (erstes Bild = automatisch Titelbild) | `profile_id`, `listing_id`, `image_url`, `category` |
| `evaluate_listing` | Inserat anhand der Profil-Kriterien bewerten (Note A–F je Kriterium + Gesamt-Score) | `profile_id`, `listing_id` |
| `propose_criteria` | Kriterium anlegen/ändern — schreibt **sofort**, kein Chat-Confirm-Card wie beim internen Tool gleichen Namens | `profile_id`, `action` (`add`/`edit`), `criterion_id` (bei edit), `name`, `description`, `weight` (1–5), `is_subjective` |
| `update_context` | Text an Globalen oder Profil-Kontext anhängen — schreibt ebenfalls sofort | `profile_id`, `scope` (`global`/`profile`), `operation` (nur `append`), `content` |
| `geocode` | Adresse zu Koordinaten auflösen (Nominatim) | `profile_id`, `address`, `country` (optional, ISO-3166-1 alpha-2) |
| `list_profile_listings` | Übersicht aller Inserate im Profil (Titel, Preis, Score, Status, Quell-URL) | `profile_id` |
| `compute_ranking` | Rangliste aller bewerteten, nicht abgelehnten/archivierten Inserate, absteigend nach Score | `profile_id` |

`propose_criteria`/`update_context` haben über MCP **kein Undo**: anders als die
gleichnamigen internen Chat-Tools (ADR 0004/0009) schreiben sie direkt, ohne
Confirm-Card — Änderungen werden als `ChangedByUser` protokolliert, nicht
`ChangedByAgent`.

### Diagnose per curl

Für Diagnosezwecke lässt sich der Handshake auch direkt mit `curl` nachvollziehen (für
den Alltag die Client-Integrationen unten verwenden):

```bash
TOKEN="<APP_PASSWORD>"
HOST="http://localhost:8080"

# 1. Session initialisieren — Antwort-Header "Mcp-Session-Id" merken
curl -sD - -o /dev/null -X POST "$HOST/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{
        "protocolVersion":"2025-06-18","capabilities":{},
        "clientInfo":{"name":"curl-test","version":"1.0"}}}'

SESSION="<aus dem Mcp-Session-Id Header oben>"

# 2. initialized-Notification bestätigen (ohne "id" = Notification)
curl -s -X POST "$HOST/mcp" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" -H "Mcp-Session-Id: $SESSION" \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}'

# 3. Tool aufrufen, z.B. list_profiles
curl -s -X POST "$HOST/mcp" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" -H "Mcp-Session-Id: $SESSION" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call",
       "params":{"name":"list_profiles","arguments":{}}}'
```

### Claude Code

```bash
claude mcp add --transport http property-finder https://<host>/mcp \
  --header "Authorization: Bearer <APP_PASSWORD>"
```

Verbindet sich direkt — Claude Code unterstützt statische Bearer-Header nativ.

### opencode

In `opencode.json`/`opencode.jsonc`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "property-finder": {
      "type": "remote",
      "url": "https://<host>/mcp",
      "oauth": false,
      "headers": {
        "Authorization": "Bearer <APP_PASSWORD>"
      }
    }
  }
}
```

`oauth: false` ist nötig, damit opencode nicht versucht, einen OAuth-Flow mit dem Server
auszuhandeln — `/mcp` kennt nur den statischen Bearer-Token.

### Claude Desktop

Claude Desktops Custom-Connector-UI akzeptiert nur OAuth-Felder, keinen statischen
Bearer-Token (ADR 0019, "Alternatives considered"). Workaround über den
Community-Proxy `mcp-remote`, den Claude Desktops stdio-Server-Unterstützung direkt
ausführen kann:

```bash
npx mcp-remote https://<host>/mcp --header "Authorization: Bearer <APP_PASSWORD>"
```

Als lokaler stdio-MCP-Server in Claude Desktops Konfiguration eintragen (Command:
`npx`, Args: `mcp-remote https://<host>/mcp --header "Authorization: Bearer <APP_PASSWORD>"`).

### Typischer Workflow über MCP

1. `list_profiles` → `profile_id` ermitteln.
2. `fetch_url` auf die Inserats-URL → Haupttext/Bilder/Links.
3. `save_listing` mit den extrahierten Feldern (neu: `listing_id` weglassen).
4. `download_image` für jedes relevante Bild.
5. `geocode` für die Adresse (füllt `latitude`/`longitude` für die Karte).
6. `evaluate_listing` → Note je Kriterium + Score.
7. `list_profile_listings` / `compute_ranking` zur Übersicht/Rangliste.
