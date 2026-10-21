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

Accepted. This affects [[../../spec/00_infrastructure_setup/00-01-tech-stack]].

## Context

The app was fully open. No auth layer existed in `backend/internal/api/router.go`. It is a single-user,
self-hosted app (see `AGENTS.md` "Project Overview"). It needs no username, no multi-user support and no
password-reset flow. It needs just one gate in front of the Frontend and the API.

Every documented deployment path sits behind an external reverse proxy that ends TLS. These paths are the
root `docker-compose.yml` (used for local dev and testing) and `deploy/docker-compose.yml` (used in
production, see `docs/deployment.md`). The container itself only speaks plain HTTP.

The session transport must work for three kinds of same-origin request that the frontend makes:
`fetch` (`src/api/http.ts`), `EventSource` for SSE (`/api/events`, used by `useChat.ts`,
`useReevaluationBanner.ts`, `useRejectionAnalysisToast.ts`), and plain `<img src>` for `/media/*`.
`EventSource` cannot set a custom `Authorization` header. Neither can an `<img>` tag. A bearer-token
scheme would need a workaround for each of them (for example, a token query parameter for both). A
same-origin cookie is sent automatically by all three.

## Decision

One `APP_PASSWORD` env var gates the whole app. `POST /api/auth/login` compares the submitted password to
`APP_PASSWORD` in constant time (`crypto/subtle.ConstantTimeCompare`). On success, it issues a
hand-written HS256 JWT (`internal/auth`). It uses no third-party JWT library. The claim set is fixed to
`iat`/`exp`, and stdlib `crypto/hmac` + `crypto/sha256` handle that in about 40 lines. This fits the
repo's habit of keeping dependencies minimal: no ORM, no DI container. The JWT is signed with
`sha256(APP_PASSWORD)`. No separate signing secret is configured. Changing the password automatically
invalidates every existing session. For a single-user app this is a wanted side effect, not a gap.

The token is stored in an httpOnly, `SameSite=Lax` cookie (`pf_session`, 30-day `MaxAge`). It is not
returned as a bearer token, for the `EventSource`/`<img>` reason above. A second, optional env var,
`COOKIE_SECURE`, sets the cookie's `Secure` flag. Its default is `true`, which matches the external
HTTPS-terminating proxy of every documented deployment. Local development without a proxy sets
`COOKIE_SECURE=false`. (`npm run dev`'s Vite proxy talks plain HTTP directly to the backend.) Without
this, the browser silently drops a `Secure` cookie sent over plain HTTP. Login then looks successful
(204), but every later request looks unauthenticated.

`RequireAuth` is a chi middleware in `internal/api/auth.go`. It guards every route except `GET /health`,
`POST /api/auth/login`, and the static assets of the embedded SPA (`router.go`). The login screen is part
of that bundle, so it must load without authentication.

## Alternatives considered

- **`Authorization: Bearer` header** — rejected. It breaks `EventSource` and `<img src="/media/...">`,
  because neither can attach a custom header. It would need a workaround for each, like a token query
  string. That would leak the token into server access logs and browser history.
- **Separate `JWT_SECRET` env var** — rejected. One env var is simpler for a single-user app. If the
  signing key comes from the password, a password change also forces a logout of every session. That is a
  feature here, not a gap, because there is exactly one legitimate user.
- **`golang-jwt/jwt` dependency** — rejected. The claim set is fixed and trivial (`iat`/`exp` only).
  Adding a third-party JWT library for this does not fit the repo's existing dependency minimalism (no
  ORM, no DI framework, stdlib `testing` only).
- **Trusting an `X-Forwarded-Proto` header from the reverse proxy** instead of an explicit `COOKIE_SECURE`
  env var — rejected. The proxy is outside this repo, and the repo does not configure it. An explicit,
  plain env var needs no trust in a proxy that this repo does not own or configure.

## Consequences

- `GET /health` must stay unauthenticated. The `wget` healthcheck in both compose files cannot
  authenticate.
- Losing or changing `APP_PASSWORD` invalidates all sessions at once (see Decision). This is fine for a
  single-user app. It would not scale to multi-user.
- There is no logout-all-devices or session-list feature. Each browser has exactly one session cookie,
  which matches the single-user scope.
- `COOKIE_SECURE=false` must be set explicitly for any plain-HTTP deployment. This includes local
  development without a reverse proxy. Otherwise login looks successful (`204` from `POST
  /api/auth/login`), but the browser silently drops the cookie. Every later request then loops back to
  `/login`.
- `EventSource` does not reconnect on its own once `/api/events` starts returning 401 after the token
  expires mid-session. The user only notices at the next navigation or API call. That call then redirects
  to `/login` through the 401 handler in `src/api/http.ts`. The TTL is 30 days and the app has one user,
  so this is an accepted edge case with no special handling.
