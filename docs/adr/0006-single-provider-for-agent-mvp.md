---
type: adr
title: "Claude API is the only provider for MVP; LM Studio and OpenCode come later"
project: property-finder-obs-style
status: accepted
created: 2026-07-28
tags:
  - property-finder-obs-style
  - adr
---

## Status

Accepted. It affects [[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Provider Abstraction]] and [[../../spec.md#Tech Stack]]. (2026-11-25: LM Studio and OpenCode are now implemented, see [[0030-openai-compatible-agent-provider]]. The MVP decision below stays as the record of why they came later.) (2026-07-29: We considered switching the POC provider to OpenCode and rejected it. Feasibility and tool-use reliability favour Claude. This decision stays as it is.)

## Context

The predecessor project needed a swappable AI provider from day one (Claude API + LM Studio). Its LLM calls were single-shot, stateless JSON round-trips, such as `ParseSearchProfile` and `EvaluateListing`. Any model that reliably returns well-formed JSON for a prompt can do those.

The Agent in this project is very different. It is a multi-turn tool-use loop. The model decides *which* tool to call and *when* to stop. It must also keep a growing conversation and two Context Files in view. Reliable, well-formed multi-turn tool use is a much higher bar than single-shot JSON. It is not yet shown that either candidate second provider meets it:

- **LM Studio** — local inference. Tool-use quality depends on the model. The user would have to pick and set up a model that handles a 9-tool agent loop reliably. We can give little guidance until real testing exists.
- **OpenCode** — an OpenAI-compatible endpoint option. Its real tool-use and streaming behaviour in the agent-loop shape of this app is unverified.

## Decision

The MVP ships with the Claude API as the only implemented provider. The `internal/agent.Provider` interface ([[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Provider Abstraction]]) exists from the start. This way business logic never depends on Claude-specific types directly. LM Studio and OpenCode get no implementation, no UI provider switcher, and no test coverage until the MVP has validated the Agent/Context/Tool model itself. `app_config.llm.provider` reserves the values `lmstudio` and `opencode`. This way the config schema needs no breaking change later.

## Alternatives considered

- **Build all three providers for MVP** — rejected. It triples what we must validate (does the agent loop work well at all). It also does this with the two least-proven parts of the stack. If the Agent/Tool model must change after real use, provider-specific work done early is wasted.
- **No abstraction at all, hard-code Claude everywhere** — rejected. It is cheap now and expensive later. The predecessor project already showed, with its own provider interface, that this seam is cheap if added from the start and costly to add afterwards.

## Consequences

- Users without Claude API access cannot use this app at MVP. This is fine, because it is a single-user personal tool and the project owner has Claude access.
- PDF native input ([[0005-pdf-native-document-input]]) is a Claude-specific feature. The Provider interface needs a capability flag before LM Studio/OpenCode support can be added. Neither is guaranteed to support the same document-input mechanism.
- Revisit trigger: once the Agent loop of the MVP is stable and proven by real use, evaluate LM Studio and OpenCode against the real Tool catalogue ([[../../spec/00_infrastructure_setup/00-03-llm-agent-integration#Tool Catalogue]]). Do not evaluate them in the abstract.
