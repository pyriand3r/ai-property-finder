# syntax=docker/dockerfile:1.7
#
# Multi-stage build. Build context is the repo ROOT (not backend/) — see
# docker-compose.yml `build: {context: ., dockerfile: backend/Dockerfile}` —
# because this needs to reach both the frontend source at the repo root and
# backend/ to embed the Vite build output into the Go binary (Layer 4.2,
# AGENTS.md "Frontend lives at repo root").
#
# Stage 1 (frontend): build the Vue SPA with Vite -> dist/.
# Stage 2 (backend):  copy dist/ into backend/internal/web/dist so the
#                     //go:embed directive picks it up, then build the binary.
# Stage 3 (runtime):  copy the single static binary onto a minimal base.
#
# Cross-platform: stages 1+2 run on the BUILD machine's native platform
# ($BUILDPLATFORM) — the SPA bundle is arch-independent and Go cross-compiles
# via GOOS/GOARCH (CGO disabled) — so `docker buildx build --platform
# linux/amd64` on an arm64 Mac needs no QEMU emulation. Only stage 3 is
# target-platform. See docs/deployment.md.

# ── Stage 1: Frontend build ────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci
COPY tsconfig*.json vite.config.ts index.html ./
COPY public/ ./public/
COPY src/ ./src/
RUN npm run build

# ── Stage 2: Backend build ──────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /app/backend
ENV CGO_ENABLED=0
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /app/dist ./internal/web/dist
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/property-finder ./cmd/server

# ── Stage 3: Runtime ─────────────────────────────────────────────────────────
FROM alpine:3.21 AS runtime
RUN apk add --no-cache wget ca-certificates tzdata && \
    adduser -D -u 10001 app
RUN mkdir -p /data && chown -R app:app /data
COPY --from=backend /out/property-finder /usr/local/bin/property-finder
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/property-finder"]
