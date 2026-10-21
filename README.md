# Property Finder OBS-Style

This is a single-user, self-hosted web app. It replaces a manual Obsidian vault workflow for cross-border property search (Sweden/Portugal/Austria). A chat-driven Agent powered by Claude does the work.

You send a listing URL, a PDF, or a screenshot (JPG/PNG) to the chat of a Search Profile. Then the Agent does these steps:

- It loads or reads the source. A PDF or image is sent as a native Claude content block (ADR 0005/0020).
- It follows portal-specific links.
- It extracts photos.
- It writes a structured Listing Document (original + German translation + evaluation).
- It gives each Criterion a grade from A to F. It also gives a weighted Score from 0 to 100.

Full project description: [docs/00_readme.md](docs/00_readme.md). Repo conventions, architecture, directory structure, and development workflow: [AGENTS.md](AGENTS.md). It is written for AI assistants. It is still the authoritative technical reference for this repo.

## Architecture

- **Backend**: Go (`backend/`, module `propertyfinder`), chi router, `pgx/v5` without an ORM. It uses SSE for real-time updates (no WebSocket).
- **Frontend**: Vue 3 SPA (`src/` in the repo root). Go serves it with `go:embed`. There is only one deployable and no separate frontend container.
- **Database**: PostgreSQL 18. Migrations run automatically at start (forward-only, fail-fast).
- **Agent**: a multi-turn Tool-use loop that does not depend on a provider (`backend/internal/agent`). Today Claude is the only provider wired in (ADR 0006/0007).
- **MCP**: the same Tool catalog is also available from outside through `POST /mcp` (ADR 0019). See [docs/operations.md](docs/operations.md#7-using-the-mcp-server).

Details and all architecture decisions: [docs/adr/](docs/adr/README.md).

## Quick start (local)

```bash
cp .env.example .env            # set DB_PASSWORD, APP_PASSWORD, CLAUDE_API_KEY
docker compose up -d --build    # backend on :8080, Postgres 18 on :5432
curl localhost:8080/health      # {"status":"ok"}
```

Then open `http://localhost:8080` and log in with `APP_PASSWORD`.

## Operations and MCP access

- Full operations guide (start/stop, configuration, backup, troubleshooting, using the MCP server with Claude Code/Claude Desktop/opencode): **[docs/operations.md](docs/operations.md)**.
- Deployment to a remote Docker host without a registry (image as a file): [docs/deployment.md](docs/deployment.md).

## Development

```bash
# Frontend (repo root)
npm run dev            # vite dev server, proxies /api + /media to :8080
npm run build           # vue-tsc -b && vite build → dist/
npm run typecheck         # vue-tsc -b --noEmit
npm run lint            # oxlint
npm run format            # oxfmt

# Backend (from backend/)
go build ./...
go test ./...
golangci-lint run           # config: backend/.golangci.yml
```

There is no CI setup. All gates (`go test`, `golangci-lint`, `npm run typecheck`, `oxlint`, `oxfmt --check`) are manual, local checks to run before a commit.

## Documentation

| File | Content |
|---|---|
| [AGENTS.md](AGENTS.md) | Repo conventions, architecture, directory structure, development commands |
| [docs/00_readme.md](docs/00_readme.md) | Project description, motivation, status |
| [docs/operations.md](docs/operations.md) | Server operations: configuration, start/stop, backup, MCP use |
| [docs/deployment.md](docs/deployment.md) | Deployment to a remote Docker host |
| [docs/adr/](docs/adr/README.md) | 21 Architecture Decision Records |
| [docs/CONTEXT.md](docs/CONTEXT.md) | Domain glossary |
| [docs/spec.md](docs/spec.md) / [docs/spec/](docs/spec/) | Project spec / feature and infrastructure specs |
| [docs/implementation-plan.md](docs/implementation-plan.md) | Build progress by layer |
| [docs/buglist.md](docs/buglist.md) / [docs/ideas.md](docs/ideas.md) | Open bugs / ideas |

## License

Copyright (C) 2026 pyriand3r. Licensed under the [GNU Affero General Public License v3.0](LICENSE) (`AGPL-3.0-only`, see [ADR 0021](docs/adr/0021-agpl-3-license.md)). If you run a changed version for others over a network (for example as a hosted service), you must offer them the full source code (§ 13).
