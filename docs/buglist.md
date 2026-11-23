---
type: tracking
title: Bug List
project: property-finder-obs-style
status: active
updated: 2026-08-06
tags:
  - property-finder-obs-style
  - bugs
---

## How to use this file

Did you find a bug while working on something else, not the thing you were asked to fix? Write
it here first. Do this before you investigate or fix it, unless the user asks you to fix it
right away. Write one entry per bug. Include the date found and the context, the symptom, the
root cause if known, the status, and links to any deeper write-up (spec, ADR).

When a bug is fixed and checked, move its entry to **Fixed**. Link the fix's commit or spec
there. Do not delete the history of what it took to find and fix it.

---

## Open

### Split-pane detail scroll falls through to page scroll near the bottom

- **Found**: 2026-08-06, reported by the user while scrolling a long Listing in the
  master-detail split ([[spec/01_mvp/01-10-listing-views-overhaul]] § Master-Detail Split
  Layout).
- **Symptom**: Scrolling inside the right-hand detail pane (`ListingDetailPanel.vue`) works
  correctly for about the first 3/4 of its content. The list and header stay in place, and only
  the detail pane scrolls. Near the end of the detail content, the scroll no longer stays
  inside the pane. The whole page starts to scroll instead.
- **Root cause (hypothesis, not confirmed)**: `.listings-panel-split-detail` in
  `ListingsPanel.vue` is `position: sticky` with `max-height: calc(100vh - 2rem)` and
  `overflow-y: auto`. This is the usual pattern for a sticky pane that scrolls on its own. But
  a `position: sticky` element can only stay stuck inside the bounds of its **containing
  block** (here, the `.listings-panel-split` flex row). The pane's own max-height is almost a
  full viewport. So its containing block ends up almost exactly as tall as the pane. That
  leaves very little scroll room between two points. The first point is when the pane's own
  `overflow-y:auto` content finishes scrolling. The second point is when the bottom edge of the
  containing block is reached, the sticky positioning gives up, and the page takes over the
  scroll. This matches the reported symptom: "fine for 3/4, then the whole page scrolls".
- **Status**: Open, not fixed. A fix probably needs to give `.listings-panel-split` a height
  tied to the viewport (not only to its content), so the sticky pane has room to stay stuck.
  Or it could move the split away from `position: sticky`. A fixed-height flex/grid shell with
  independent scroll regions is a more typical split-pane layout.

### Quell-URL link color hard to read in dark mode

- **Found**: 2026-08-06, reported by the user after the clickable source link shipped
  ([[spec/01_mvp/01-10-listing-views-overhaul]] § Detail View: Clickable Source Link).
- **Symptom**: The `<a>` tag that shows `listing.source_url` in view mode (`ListingForm.vue`)
  has no explicit color style. It uses the browser's default link blue. That blue is dark and
  has low contrast on this app's dark theme.
- **Root cause**: The Quell-URL `<a>` in `ListingForm.vue` was added with only
  `target="_blank" rel="noopener"`. It has no class or style, so it was never themed. The app
  already has a dark/light theme setup (`usePreferredDark` composable,
  `NConfigProvider :theme`). Other components style against it (for example, `var(--n-color-target, ...)`
  fallbacks in `ListingListTable.vue`/`RankingView.vue`). This link never got the same
  treatment.
- **Status**: Open, not fixed.

---

## Fixed

### Criterion names show literal `&amp;` instead of `&`

- **Found**: 2026-08-06, while live-checking the Detail View grade grid
  ([[spec/01_mvp/01-10-listing-views-overhaul]])
- **Symptom**: Criterion names with an ampersand showed as `Grundstück &amp; Lage`. The literal
  `&amp;` was already in the raw `GET /api/profiles/{id}/criteria` response.
- **Root cause** (issue #4): no backend code escapes HTML (no `html.EscapeString`, no templating on
  this path). The model itself wrote the entity into the `propose_criteria` argument, and
  `ToolCallService.confirmProposeCriteria` stored it as it was. The REST form and MCP paths store
  what the user or client sends.
- **Fix** (issue #4): `parseCriteriaProposal` (`internal/service/tool_call_service.go`) runs
  `html.UnescapeString` on name and description when a proposal is confirmed. This covers live
  Agent turns and Rejection Analysis proposals, since both are confirmed through that function.
- **Existing rows**: no migration. It is a single-user app with a handful of criteria, and the
  migration SQL cannot be tested here (no DB tests exist). Fix old rows by editing the name in the
  Criteria form, or once with
  `UPDATE criterion SET name = replace(name, '&amp;', '&'), description = replace(description, '&amp;', '&');`.
- **Known gap**: a pending Confirm-Card still shows the raw `&amp;` until confirmed, because the
  stored `tool_calls` input is not rewritten. The confirmed criterion is correct.

### Stale "Bewertungstabelle" placeholder at the bottom of the Listing Document

- **Found**: 2026-08-06, reported by the user while reviewing the Detail View after
  [[spec/01_mvp/01-10-listing-views-overhaul]] shipped.
- **Symptom**: At the very bottom, the Detail View still shows a "Bewertungstabelle" heading
  with the placeholder text "(wird nach Kriterien-Bewertung durch evaluate_listing ergänzt)"
  (means: "will be added after criteria grading by evaluate_listing"). It sits inside the
  rendered `document_md` (`ListingDocument.vue`). This is the case even though the real,
  structured grade grid (`ListingGradeGrid.vue`) now shows at the *top* of the panel, since the
  "Grade Table First" change in 01-10.
- **Root cause (likely)**: `document_md` is prose written by the Agent. By design, it contains
  its own "## Bewertungstabelle" section. See the system prompt instructions in
  `backend/internal/agent/context_builder.go` (lines ~35, ~70) and the seeded global context
  default in `backend/internal/db/migrations/000002_seed_global_context_defaults.up.sql` ("-
  Bewertungstabelle pro Kriterium"). That embedded section was always meant to be *the* grade
  display, before 01-10 added the separate structured `ListingGradeGrid.vue`, which is fed from
  `listing_criterion_grade` rows. 01-10 moved the structured grid to the top. But it never
  changed the Agent's document-generation instructions to drop its own embedded copy (or to
  stop filling it in). Now there are two Bewertungstabelle sections. One is either live or a
  stale placeholder. The other is probably redundant with the real grid once the listing is
  graded.
- **Status**: Fixed in #5 (branch `bugfix/5-stale-placeholder`). Frontend only: `ListingDocument.vue`
  hides the "Bewertungstabelle" section (heading up to the next heading of the same or higher level)
  in the rendered view. This also covers documents that are already stored. The stored `document_md`
  and the edit view are not changed. The Agent prompt still tells the Agent to write this section
  (`context_builder.go`, `tools.go`, seeded global context). That wasted text is not removed here.
  Not verified in a browser.

### Unicode escape sequences displayed literally in Confirm-Card

- **Found**: reported during the Layer 6 review. Write-up:
  [[spec/01_mvp/01-12-chat-feedback-fixes]] § Unicode Escape Sequences Displayed Literally
- **Symptom**: Once, a Confirm-Card showed literal `\uXXXX` instead of umlauts.
- **Fix** (issue #6): the bug never reproduced live, so the fix is defensive. `ChatConfirmCard.vue`
  now runs name, description and content through `decodeUnicodeEscapes` (`src/utils/chat.ts`).
  This covers a double-escaped string value, the only way a standard `JSON.parse` result can still
  contain `\uXXXX`.
- **Known gap**: the display is decoded, but confirming still writes the stored value
  (`ToolCallService.confirmProposeCriteria` / `confirmUpdateContext`). A double-escaped value would
  be saved with literal `\uXXXX`. Not fixed here. Fix it in the backend if it is ever seen live.

### A chat session's history can end up with a dangling `tool_use` again, breaking every future turn in it

- **Found**: 2026-08-07, while live-checking Layer 8.2 (Listing-Context-Block). A new message
  was sent in an existing active session ("Schweden" profile, session
  `e278bb0a-45f7-41e3-a748-76656f1cba0d`).
- **Symptom**: The turn fails with a 400 from the Claude API:
  `messages.10: tool_use ids were found without tool_result blocks immediately after`. Every
  later message sent to that same session fails the same way. The session is stuck for good
  until "Neue Session" is used, because the invalid history is replayed on every turn.
- **Confirmed via `docker compose logs`**: `agent: turn failed for session ...: POST
  "https://api.anthropic.com/v1/messages": 400 Bad Request ... tool_use ids were found without
  tool_result blocks immediately after: 20af33ee-f72b-4ac8-95bc-50b8f763d401`.
- **Root cause (original note)**: Not investigated yet. The *symptom* is the same as the bug that
  [[spec/01_mvp/01-09-chat-reliability-fixes]] fixed (RF.1 — dangling `tool_use` blocks from
  the budget-exhausted branch of `loop.go`). But that fix only targeted the path where the
  roundtrip limit is hit. This session had already had many ad-hoc live-test turns (fetch_url
  403s, geocode, download_image validation errors, etc.). The dangling call could come from a
  different path that is still not fixed. In that path, a tool_use is saved without ever being
  dispatched, or without its result being saved (for example, a turn that failed or was
  interrupted in the middle of dispatch). Or it could be old data from before RF.1 shipped,
  which was simply never replayed until now. A fresh repro is needed to tell which.
- **Root cause (found 2026-11-23, issue #1)**: `AgentService.persistRoundtrip` wrote the assistant
  row (with `tool_calls`) and then each `role=tool` row as separate INSERTs, and only logged a
  failed INSERT. Any stop between them left a stored `tool_use` without a result: a server
  restart (ADR 0008, no recovery), a tool INSERT that failed (for example a result that is not
  valid JSON for the JSONB column), or two turns interleaving in one session. A `tool_use` input
  cut off mid-JSON made `toolCallsJSON` return nothing, which gave the reverse (orphan
  `tool_result`). Nothing in the loop's dispatch path builds this in memory: `Run` only calls
  `onRoundtrip` after dispatch.
- **Status**: Fixed in #1 (branch `bugfix/1-dangling-tool-use`). Two parts, see
  [[adr/0023-chat-history-repair-and-atomic-persist]] and
  [[spec/01_mvp/01-09-chat-reliability-fixes]]: `agent.RepairToolPairing` repairs the history on
  every load (this also heals sessions that are already broken), and each roundtrip is now
  persisted in one transaction with always-valid JSON. Tests: `internal/agent/repair_test.go`,
  `TestToolResultJSON_*` / `TestToolCallsJSON_TruncatedToolInput*` in
  `internal/service/agent_service_test.go`, `TestFormatMessages_RepairedHistory*` in the Claude
  provider tests. Not verified against the live Claude API.
