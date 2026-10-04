# Property Finder OBS-Style

Single-user, self-hosted Web-App, die einen manuellen Obsidian-Vault-Workflow für
grenzüberschreitende Immobiliensuche (Schweden/Portugal/Österreich) durch einen
chat-gesteuerten, Claude-gestützten Agenten ersetzt. Eine Inserats-URL, ein PDF oder ein
Screenshot (JPG/PNG) geht in den Chat eines Such-Profils; der Agent lädt/liest die Quelle
(bzw. verarbeitet PDF/Bild als natives Claude-Content-Block, ADR 0005/0020), folgt
portalspezifischen Links, extrahiert Fotos, schreibt ein strukturiertes
Listing-Dokument (Original + deutsche Übersetzung + Bewertung) und vergibt pro
Kriterium eine Note A–F sowie einen gewichteten 0–100-Score.

Vollständige Projektbeschreibung: [docs/00_readme.md](docs/00_readme.md). Repo-Konventionen,
Architektur, Verzeichnisstruktur, Entwicklungs-Workflow: [AGENTS.md](AGENTS.md) (für
KI-Assistenten geschrieben, aber die maßgebliche technische Referenz für dieses Repo).

## Architektur

- **Backend**: Go (`backend/`, Modul `propertyfinder`), chi-Router, `pgx/v5` ohne ORM,
  SSE für Echtzeit-Updates (kein WebSocket).
- **Frontend**: Vue 3 SPA (Repo-Wurzel `src/`), von Go per `go:embed` ausgeliefert — ein
  einziges Deployable, kein separater Frontend-Container.
- **Datenbank**: PostgreSQL 18, Migrationen laufen beim Start automatisch (forward-only,
  fail-fast).
- **Agent**: provider-unabhängiger Multi-Turn-Tool-Use-Loop (`backend/internal/agent`),
  aktuell mit Claude als einzigem Provider verdrahtet (ADR 0006/0007).
- **MCP**: derselbe Tool-Katalog ist zusätzlich extern über `POST /mcp` erreichbar
  (ADR 0019) — siehe [docs/operations.md](docs/operations.md#7-mcp-server-nutzung).

Details und alle Architekturentscheidungen: [docs/adr/](docs/adr/README.md).

## Schnellstart (lokal)

```bash
cp .env.example .env            # DB_PASSWORD, APP_PASSWORD, CLAUDE_API_KEY setzen
docker compose up -d --build    # Backend auf :8080, Postgres 18 auf :5432
curl localhost:8080/health      # {"status":"ok"}
```

Anschließend `http://localhost:8080` öffnen und mit `APP_PASSWORD` einloggen.

## Betrieb & MCP-Zugriff

- Ausführliche Betriebsanleitung (Start/Stop, Konfiguration, Backup, Troubleshooting,
  MCP-Server-Nutzung für Claude Code/Claude Desktop/opencode): **[docs/operations.md](docs/operations.md)**.
- Deployment auf einen entfernten Docker-Host ohne Registry (Image als Datei):
  [docs/deployment.md](docs/deployment.md).

## Entwicklung

```bash
# Frontend (Repo-Wurzel)
npm run dev            # vite dev server, proxied /api + /media auf :8080
npm run build           # vue-tsc -b && vite build → dist/
npm run typecheck         # vue-tsc -b --noEmit
npm run lint            # oxlint
npm run format            # oxfmt

# Backend (aus backend/)
go build ./...
go test ./...
golangci-lint run           # Config: backend/.golangci.yml
```

Es gibt kein CI-Setup — alle Gates (`go test`, `golangci-lint`, `npm run typecheck`,
`oxlint`, `oxfmt --check`) sind manuelle/lokale Checks vor einem Commit.

## Dokumentation

| Datei | Inhalt |
|---|---|
| [AGENTS.md](AGENTS.md) | Repo-Konventionen, Architektur, Verzeichnisstruktur, Entwicklungsbefehle |
| [docs/00_readme.md](docs/00_readme.md) | Projektbeschreibung, Motivation, Status |
| [docs/operations.md](docs/operations.md) | Server-Betrieb: Konfiguration, Start/Stop, Backup, MCP-Nutzung |
| [docs/deployment.md](docs/deployment.md) | Deployment auf einen entfernten Docker-Host |
| [docs/adr/](docs/adr/README.md) | 20 Architecture Decision Records |
| [docs/CONTEXT.md](docs/CONTEXT.md) | Domänen-Glossar |
| [docs/spec.md](docs/spec.md) / [docs/spec/](docs/spec/) | Projekt-Spec / Feature-&-Infrastruktur-Specs |
| [docs/implementation-plan.md](docs/implementation-plan.md) | Baufortschritt nach Layer |
| [docs/buglist.md](docs/buglist.md) / [docs/ideas.md](docs/ideas.md) | Offene Bugs / Ideen |
