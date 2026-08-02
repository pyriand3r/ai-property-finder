---
type: adr
title: "Claude API is the sole provider for MVP; LM Studio and OpenCode deferred"
project: property-finder-obs-style
status: accepted
created: 2026-07-28
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted — affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Provider Abstraction]], [[../../spec.md#Tech Stack]]. (2026-07-29: Wechsel des POC-Providers zu OpenCode wurde erwogen und verworfen — Machbarkeit/Tool-Use-Zuverlässigkeit sprechen für Claude; diese Entscheidung bleibt unverändert bestehen.)

## Context

dev-property-finder required a swappable AI provider from day one (Claude API + LM Studio) because its LLM calls were single-shot, stateless JSON round-trips — `ParseSearchProfile`, `EvaluateListing`. Any model that reliably returns well-formed JSON for a given prompt can serve those. This project's Agent is materially different: a multi-turn tool-use loop where the model decides *which* tool to call, *when* to stop, and has to hold a growing conversation plus two Context Files in view while doing it. Reliable, well-formatted multi-turn tool use is a meaningfully higher bar than single-shot JSON, and it's not yet established that either candidate second provider clears it:

- **LM Studio** — local inference, model-dependent tool-use quality; the burden of picking and configuring a model that handles a 9-tool agent loop reliably would fall on the user, with little guidance possible until real testing exists
- **OpenCode** — an OpenAI-compatible endpoint option; its actual tool-use/streaming behaviour in this app's agent-loop shape is unverified

## Decision

MVP ships with Claude API as the only implemented provider. The `internal/agent.Provider` interface ([[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Provider Abstraction]]) exists from the start so business logic never depends on Claude-specific types directly, but LM Studio and OpenCode get no implementation, no UI provider-switcher, and no test coverage until after MVP validates the Agent/Context/Tool model itself. `app_config.llm.provider` reserves the values `lmstudio` and `opencode` so the config schema doesn't need a breaking change later.

## Alternatives considered

- **Build all three providers for MVP** — rejected: triples the surface to validate (does the agent loop even work well) with the two least-proven parts of the stack; if the Agent/Tool model itself needs to change after real usage, provider-specific work done early is wasted
- **No abstraction at all, hard-code Claude everywhere** — rejected: cheap now, expensive later — dev-property-finder already proved (via its own provider interface) that keeping this seam is low-cost when done from the start and expensive to retrofit

## Consequences

- Users without Claude API access cannot use this app at MVP — acceptable given it's a single-user personal tool for the project owner, who has Claude access
- PDF native-input ([[0005-pdf-native-document-input]]) is a Claude-specific capability; the Provider interface needs a capability flag before LM Studio/OpenCode support can be added, since neither is guaranteed to support the same document-input mechanism
- Revisit trigger: once MVP's Agent loop is stable and validated by real use, evaluate LM Studio and OpenCode against the actual Tool catalogue ([[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]]) rather than in the abstract
