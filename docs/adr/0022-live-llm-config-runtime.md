---
type: adr
title: "LLM settings are editable in the UI and applied live through an atomic runtime holder"
project: property-finder-obs-style
status: accepted
created: 2026-11-23
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. Builds on [[0006-single-provider-for-agent-mvp]], [[0007-provider-independent-agent-loop]] and
[[0013-agent-turn-llm-roundtrip-budget]]. Closes issue #23.

## Context

The Tier-2 settings (`llm.*` in `app_config`) could only be read (`GET /api/config`). Changing the model,
API key or roundtrip budget meant editing the DB by hand. Both providers (`ai.Provider` and
`agent.Provider`) were built once at boot, so even a DB edit needed a restart.

## Decision

- `PATCH /api/config` takes a partial `{"llm": {...}}` body. Every field is optional. Bad values return a
  `domain.ValidationError` (HTTP 400, German message). The values are saved to `app_config` in one
  statement (`config.SaveLLM`).
- `internal/llm.Runtime` owns the live configuration. It holds an immutable `Snapshot` (config + both
  providers) behind an `atomic.Pointer`. A write builds new providers, saves, and only then swaps the
  pointer. Writers are serialized. Readers never lock.
- `Runtime` implements `ai.Provider` by delegating to the current snapshot. Evaluation, ranking and
  rejection analysis therefore use the new model on their next call.
- `AgentService` takes one `Snapshot` at the start of each turn and uses it for the whole turn: provider,
  message formatting and the roundtrip budget. A settings change applies from the next turn. A running
  turn is not affected.
- The API key is write-only. `GET`/`PATCH` return only `api_key_configured`. An update without `api_key`
  keeps the stored key. An empty `api_key` is rejected. `config.LLM` redacts the key in `String()` and
  `GoString()`.
- `GET /api/config` also returns `providers`: the backend-owned list of implemented providers with model
  suggestions (`llm.Providers()`). The Settings UI (`/settings`) renders its pickers from it. Models are
  suggestions only: any non-empty model ID is accepted.
- The merge base for a PATCH is reloaded from the DB, not from memory. The DB stays authoritative.
  `SeedFromEnv` is unchanged (first boot only).

## Alternatives considered

- **Restart required.** Simplest, but changing a model would need shell access. Rejected.
- **A delegating `agent.Provider` wrapper.** A swap between `FormatMessages` and `Chat` of one turn could mix
  two configurations. The per-turn snapshot avoids this.
- **Strict model allow-list in the backend.** Goes stale as soon as a new model is released. Rejected.

## Consequences

- Edits made directly in SQL are not picked up by the running process. The next `PATCH` merges on top of
  them. A restart also loads them.
- Adding a provider means adding an entry to `llm.Providers()` and a branch in
  `llm.buildSnapshot`. No frontend change beyond the generic Base URL / API-key hints (LM Studio and
  OpenCode were added this way, ADR 0030).
- The `ai.Provider` instance inside a `Snapshot` is rebuilt on every change. This is cheap (SDK client
  construction only).
