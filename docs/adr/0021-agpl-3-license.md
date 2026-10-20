---
type: adr
title: "The project is published under AGPL-3.0-only"
project: property-finder-obs-style
status: accepted
created: 2026-10-20
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. No dependency on earlier ADRs.

## Context

The repository is being published on GitHub. The author wanted the strongest protection against
closed-source reuse and hosted resale that still counts as open source (OSI-approved).

## Decision

The code is licensed under **GNU Affero General Public License v3.0 only** (SPDX: `AGPL-3.0-only`).
The full text is `LICENSE` at the repo root; `package.json` carries `"license": "AGPL-3.0-only"`.

- Derivative works must be released under AGPL-3.0 with complete source.
- §13 (network clause): anyone who modifies the app and lets users interact with it over a network
  (e.g. a hosted SaaS) must offer those users the modified source.
- "only", not "or-later": the author keeps control over whether a future AGPL version applies.

## Alternatives considered

- **PolyForm Noncommercial / Strict** — the only options that actually forbid commercial use, selling
  and paid SaaS. Rejected: not OSI-approved, so the project could not be called open source.
- **GPL-3.0** — rejected: no network clause, so a hosted modified copy needs no source release.
- **MIT / Apache-2.0** — rejected: permit closed-source reuse and resale.
- **SSPL / BSL / Elastic License 2.0** — rejected: not OSI-approved.

## Consequences

- AGPL does **not** prohibit commercial use, selling or paid hosting; it only requires that
  modifications stay open. That is inherent to every OSI-approved license (Open Source Definition,
  clauses 5 and 6).
- Third-party dependencies (Naive UI, OpenLayers, `anthropic-sdk-go`, chi, pgx, …) use permissive
  licenses compatible with AGPL-3.0; their notices must stay intact in distributed builds.
- A commercial dual license can be sold later, but only while the author holds all copyright. Accepting
  outside contributions would then require a CLA or copyright assignment first.
- Users running a modified instance publicly must provide a source link to its users.
