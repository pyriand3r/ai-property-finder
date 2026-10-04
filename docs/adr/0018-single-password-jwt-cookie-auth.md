---
type: adr
title: "Single-password login, HS256 JWT in an httpOnly cookie"
project: property-finder-obs-style
status: accepted
created: 2026-08-15
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — affects [[../../spec/00_infrastructure_setup/00-01-tech-stack]].

## Context

The app was fully open — no auth layer existed in `backend/internal/api/router.go`. It's a single-user,
self-hosted app (see `AGENTS.md` "Project Overview"): no username, no multi-user support, no
password-reset flow needed, just a single gate in front of Frontend and API.

Every documented deployment path — the root `docker-compose.yml` used for local dev/testing and
`deploy/docker-compose.yml` used in production (`docs/deployment.md`) — sits behind an external
TLS-terminating reverse proxy; the container itself only ever speaks plain HTTP.

The session transport has to work for three different kinds of same-origin request the frontend makes:
`fetch` (`src/api/http.ts`), `EventSource` for SSE (`/api/events`, used by `useChat.ts`,
`useReevaluationBanner.ts`, `useRejectionAnalysisToast.ts`), and plain `<img src>` for `/media/*`.
`EventSource` cannot set a custom `Authorization` header, and neither can an `<img>` tag — a bearer-token
scheme would need a per-consumer workaround (e.g. a token query parameter for both). A same-origin cookie
is sent automatically by all three.

## Decision

One `APP_PASSWORD` env var gates the whole app. `POST /api/auth/login` compares the submitted password to
`APP_PASSWORD` in constant time (`crypto/subtle.ConstantTimeCompare`) and, on success, issues a
hand-rolled HS256 JWT (`internal/auth`, no third-party JWT library — the claim set is fixed to `iat`/`exp`,
which stdlib `crypto/hmac` + `crypto/sha256` handles in ~40 lines, consistent with this repo's
dependency-minimalism precedent: no ORM, no DI container). The JWT is signed with `sha256(APP_PASSWORD)` —
no separate signing secret is configured; changing the password automatically invalidates every existing
session, which is a desired side effect for a single-user app, not a gap.

The token is stored in an httpOnly, `SameSite=Lax` cookie (`pf_session`, 30-day `MaxAge`) rather than
returned as a bearer token, for the `EventSource`/`<img>` reason above. The cookie's `Secure` flag is
driven by a second, optional env var, `COOKIE_SECURE` (default `true`, matching every documented
deployment's external HTTPS-terminating proxy); local development without a proxy in front (`npm run
dev`'s Vite proxy talks plain HTTP directly to the backend) sets `COOKIE_SECURE=false` — otherwise the
browser silently discards a `Secure` cookie sent over plain HTTP, and login appears to succeed (204) while
every subsequent request looks unauthenticated.

`RequireAuth`, a chi middleware in `internal/api/auth.go`, gates every route except `GET /health`, `POST
/api/auth/login`, and the embedded SPA's static assets (`router.go`) — the login screen itself is part of
that bundle and must load unauthenticated.

## Alternatives considered

- **`Authorization: Bearer` header** — rejected: breaks `EventSource` and `<img src="/media/...">`, neither
  of which can attach a custom header; would need a per-consumer workaround like a token query string,
  which leaks the token into server access logs and browser history.
- **Separate `JWT_SECRET` env var** — rejected: one env var is simpler for a single-user app, and deriving
  the signing key from the password means a password rotation is also a forced logout of every session — a
  feature here, not a gap, since there's exactly one legitimate user.
- **`golang-jwt/jwt` dependency** — rejected: the claim set is fixed and trivial (`iat`/`exp` only); adding
  a third-party JWT library for this is inconsistent with the repo's existing dependency-minimalism (no
  ORM, no DI framework, stdlib `testing` only).
- **Trusting an `X-Forwarded-Proto` header from the reverse proxy** instead of an explicit `COOKIE_SECURE`
  env var — rejected: the proxy is external to this repo and unconfigured by it; an explicit, boring env
  var needs no trust assumption about a proxy this repo doesn't own or configure.

## Consequences

- `GET /health` must stay unauthenticated — both compose files' `wget` healthcheck has no way to
  authenticate.
- Losing or rotating `APP_PASSWORD` invalidates all sessions immediately (see Decision) — acceptable for a
  single-user app, would not scale to multi-user.
- No logout-all-devices or session-list feature exists — there's exactly one session cookie per browser,
  matching the single-user scope.
- `COOKIE_SECURE=false` must be set explicitly for any plain-HTTP deployment, including local development
  without a reverse proxy in front — otherwise login looks like it succeeds (`204` from `POST
  /api/auth/login`) while the browser silently drops the cookie and every subsequent request loops back to
  `/login`.
- `EventSource` does not auto-reconnect once `/api/events` starts returning 401 after token expiry
  mid-session; the user only notices on their next navigation or API call, which then redirects to
  `/login` via `src/api/http.ts`'s 401 handler. Given the 30-day TTL on a single-user app, this is an
  accepted edge case, not specially handled.
