---
type: plan
created: 2026-08-02
updated: 2026-08-02
title: Property Finder OBS-Style — Implementation Plan
project: property-finder-obs-style
tags:
  - property-finder-obs-style
  - implementation-plan
  - checklist
---

## Prinzip

Jeder Step produziert etwas **manuell Testbares** — curl, Browser, DB-Query. Kein Step hängt von einem späteren ab. Nach jedem Step: testen, nachjustieren, erst dann weiter.

Layer 5 macht die App **vor dem Agenten** als manuellen Finder mit AI-Scoring komplett nutzbar (Form-Fallback, ADR 0001). Danach ist jeder Agenten-Output ab Layer 6 sofort im echten UI prüfbar statt nur per DB-Query.

---

## Layer 0: Skelett

- [x] **0.1 — Docker Compose + DB**
  - `docker-compose.yml`: PostgreSQL 18 + Go-Backend (leer, health endpoint)
  - DB-Migration: alle Tabellen aus `00-02-database-schema.md` anlegen
  - Volume `pgdata`, Volume `./data:/data`
  - **Test:** `docker-compose up` → `curl localhost:8080/health` → `200 OK`
  - **Test:** `psql -h localhost -U app -d propertyfinder -c "\dt"` → alle Tabellen sichtbar

- [x] **0.2 — Go-Projektstruktur + Config**
  - `backend/` mit `go.mod` (Module-Pfad: `propertyfinder`), `cmd/server/main.go`
  - Linting: `backend/.golangci.yml` existiert bereits — `golangci-lint run` muss ab dem ersten Commit sauber durchlaufen
  - `internal/config/` — laedt `app_config` aus DB
  - `internal/db/` — Verbindungspool, Migration-Runner
  - API-Router (chi) mit `/health`
  - Global-Context-Seed in Migration (Default-Text aus `01-02`)
  - **Test:** `curl localhost:8080/api/config` → liefert `llm.*`-Defaults
  - **Test:** `psql -c "SELECT content_md FROM context WHERE scope='global'"` → Default-Text vorhanden

---

## Layer 1: Profil + Context + Kriterien (manuell, kein AI)

- [x] **1.1 — Search Profile CRUD**
  - Endpoints: `POST /api/profiles`, `GET /api/profiles`, `PATCH /api/profiles/{id}`, `DELETE /api/profiles/{id}`
  - `PATCH /api/profiles/{id}/archive` (setzt `archived_at`)
  - Beim Create: automatisch `chat_session` + leeren Profile-Context anlegen
  - **Test:** `curl -X POST .../api/profiles -d '{"name":"Schweden"}'` → `201` mit ID
  - **Test:** `curl .../api/profiles` → zeigt "Schweden"
  - **Test:** `curl -X PATCH .../api/profiles/{id}/archive` → in DB `archived_at` gesetzt

- [x] **1.2 — Context Files (manuell editieren)**
  - Endpoints: `GET /api/context/global`, `PUT /api/context/global`, `GET /api/profiles/{id}/context`, `PUT /api/profiles/{id}/context`
  - `context_revision`-Row bei jedem `PUT` schreiben (changed_by = 'user')
  - `GET /api/profiles/{id}/context/history` → Liste der Revisionen
  - **Test:** `curl -X PUT .../api/profiles/{id}/context -d '{"content_md":"Bei Booli: Makler-Link folgen"}'` → `200`
  - **Test:** `curl .../api/profiles/{id}/context/history` → eine Revision sichtbar

- [x] **1.3 — Criterion CRUD (manuell)**
  - Endpoints: `GET /api/profiles/{id}/criteria`, `POST /api/profiles/{id}/criteria`, `PATCH /api/criteria/{id}`, `DELETE /api/criteria/{id}`
  - Weight 1-5, Validierung
  - Kein LLM-Call — rein manuell
  - Hook im Write-Path (Listing-Tabelle existiert seit 0.1, testbar ab 2.3): **Weight-only-Aenderung** → pure-SQL Score-Recompute ueber alle Profil-Listings (kein Flag, kein LLM). **Jede andere Aenderung** (Name, Beschreibung, Objective/Subjective, Add, Delete) → `needs_reevaluation = true` auf allen Listings des Profils
  - **Test:** `curl -X POST .../api/profiles/{id}/criteria -d '{"name":"Lage","is_objective":false,"weight":4}'` → `201`
  - **Test:** `curl .../api/profiles/{id}/criteria` → zeigt "Lage", Weight 4

---

## Layer 2: Listing + Bewertung (manuell + Single-Shot-LLM)

- [x] **2.1 — Listing CRUD (manuell)**
  - Endpoints: `POST /api/profiles/{id}/listings`, `GET /api/profiles/{id}/listings`, `GET /api/listings/{id}`, `PATCH /api/listings/{id}`, `DELETE /api/listings/{id}`
  - Alle Felder aus Schema
  - `PATCH` auf strukturiertem Feld → `needs_reevaluation = true` **und** Feldname an `user_confirmed_fields` anhaengen (noetig fuer 8.2)
  - `DELETE` → erst `os.RemoveAll("/data/images/listings/{id}")`, dann DB
  - **Test:** `curl -X POST .../api/profiles/{id}/listings -d '{"title":"Kappmuren 2","price":310000}'` → `201`
  - **Test:** `curl -X PATCH .../api/listings/{id} -d '{"price":305000}'` → DB: `needs_reevaluation = true`, `user_confirmed_fields = {price}`

- [x] **2.2 — evaluate_listing (Single-Shot-LLM-Call)** — code complete, unit-tested; live Claude API call not yet run (no key in dev sandbox) — verify with `POST /api/listings/{id}/evaluate` once `CLAUDE_API_KEY`/`llm.model` are set
  - `POST /api/listings/{id}/evaluate` — kein Agent, kein Tool-Use
  - Prompt: Criterion-Liste + Listing-Daten → Grades mit Reasoning + Score
  - Provider-Interface wie dev-property-finder: `Evaluate(ctx, criteria, listing) -> ([]Grade, score, error)`
  - `needs_reevaluation = false` bei Erfolg
  - **Model:** Claude 3.5 Sonnet · GPT-4o-mini (Single-Shot, strukturiertes JSON — Haiku wuerde reichen, aber Sonnet ist sicherer bei Edge-Cases)
  - **Test:** Criterion anlegen (1.3), Listing anlegen (2.1), `curl -X POST .../api/listings/{id}/evaluate` → DB: `overall_score > 0`, `listing_criterion_grade`-Rows existieren

- [x] **2.3 — needs_reevaluation + Bulk-Trigger** — SSE/task plumbing fully verified; the underlying LLM scoring itself has the same caveat as 2.2 (no `CLAUDE_API_KEY` in dev sandbox)
  - `GET /api/profiles/{id}/listings?needs_reevaluation=true` → nur stale Listings
  - `POST /api/profiles/{id}/listings/reevaluate-all` → sequenzielle Goroutine, SSE-Stream (Task-SSE-Plumbing, wird in 6.3 fuer Chat-SSE wiederverwendet)
  - **Test:** Criterion-Beschreibung aendern → `needs_reevaluation = true` auf allen Listings → `curl .../reevaluate-all` → SSE-Stream zeigt Fortschritt → Scores aktualisiert
  - **Test:** Criterion-Weight aendern → Scores sofort neu per SQL, Flag bleibt `false` (kein LLM-Call)

---

## Layer 3: File-Handling (Bilder, PDF, Geocoding)

- [ ] **3.1 — Image Upload + Storage + Serving**
  - `/media/` → `http.FileServer(http.Dir("/data"))` im chi-Router
  - `POST /api/listings/{id}/images` — Multipart-Upload, UUID-rename
  - `GET /api/listings/{id}/images` — Liste aller `listing_image`-Rows
  - `DELETE /api/listings/{id}/images/{image_id}` — Datei + DB-Row loeschen
  - **Test:** `curl -F "file=@photo.jpg" .../api/listings/{id}/images` → `201` → `curl .../media/images/listings/{id}/{uuid}.jpg` → Bild sichtbar

- [ ] **3.2 — Title Image setzen/aendern**
  - `PATCH /api/listings/{id}` mit `{"title_image_id":"..."}`
  - Erstes Image-Upload: `title_image_id` automatisch setzen
  - **Test:** Zwei Bilder hochladen → `title_image_id` auf erstes → per PATCH auf zweites wechseln

- [ ] **3.3 — PDF Upload**
  - `POST /api/upload/pdf` — Multipart, max 32 MB, speichert Temp-Datei
  - **Test:** `curl -F "file=@listing.pdf" .../api/upload/pdf` → `201` mit Pfad

- [ ] **3.4 — Geocoding (Nominatim)**
  - `internal/geocode/nominatim.go` — HTTP GET
  - `GET /api/geocode?address=...&country=...` → `{"lat","lon","display_name"}`
  - Rate-Limiting: Mutex + Last-Call-Timestamp
  - **Test:** `curl ".../api/geocode?address=Kappmuren+Malmo"` → Koordinaten

---

## Layer 4: Chat-Basis + Frontend-Skelett

- [ ] **4.1 — Chat Session + Message Persistence**
  - Session-CRUD, Message-CRUD (nur User-Nachrichten, noch kein Agent)
  - `POST /api/profiles/{id}/chat/sessions/{sid}/messages` speichert nur
  - `POST /api/profiles/{id}/chat/sessions` legt neue Session an: alte Session bekommt `ended_at` gesetzt (read-only), neue Row ist aktiv
  - **Test:** Nachricht senden → persistiert → Reload → noch da
  - **Test:** Zweite Session anlegen → alte hat `ended_at` gesetzt, beide in DB

- [ ] **4.2 — Frontend-Skelett + Profil-Liste (Vue)**
  - Scaffolding: Vite + TypeScript + Vue Router + Naive UI; Go embedt `dist/` (Single-Binary), Dev-Proxy auf `:8080`
  - `npm add -D oxlint oxfmt` — Configs (`.oxlintrc.json`, `.oxfmtrc.json`) liegen bereits im Repo-Root; npm-Scripts `lint`/`format`/`typecheck` anlegen. **Kein** ESLint/Prettier.
  - `ProfileListView.vue` — Liste, Archiv, Create/Rename/Archive
  - Klick auf Profil → Profil-Shell mit Tab-Strip (Platzhalter-Tabs)
  - **Test:** Browser → Profil anlegen → anklicken → Profil-Shell erscheint

---

## Layer 5: Manuelle Views (Form-Fallback komplett nutzbar)

Macht die App vor dem Agenten als manuellen Finder mit AI-Scoring nutzbar. Nutzt nur die fertigen REST-APIs aus Layer 1-3.

- [ ] **5.1 — Criterion Editor**
  - Liste aller Criteria mit Weight-Slider, Add/Edit/Delete
  - Weight-Aenderung → Scores aktualisieren sofort (SQL-Recompute), kein Banner
  - Andere Aenderungen → Banner "N Inserate neu bewerten" + "Alle neu bewerten"-Button (2.3)
  - **Test:** Weight aendern → Scores neu, kein Banner → Beschreibung aendern → Banner erscheint → "Alle neu bewerten" klicken → SSE-Fortschritt

- [ ] **5.2 — List View + Ranking View**
  - Sortierbare/filterbare Tabelle (Score, Preis, Groesse, Datum; Filter Status/Score/Preis)
  - Ranking: Score-Sortierung + Kurzfazit + Gesamt-Summary (`search_profile.ranking_summary`)
  - "Rangliste aktualisieren"-Button: isolierter Single-Shot-LLM-Call (kein Agent, nutzt Provider-Seam aus 2.2), speichert `listing.kurzfazit` + `search_profile.ranking_summary`
  - **Model:** Claude 3.5 Sonnet · GPT-4o-mini (Text-Generierung, kein Tool-Use — Haiku reicht fuer Kurztexte)
  - **Test:** Mehrere Listings → List View zeigt alle → Ranking View sortiert → "Rangliste aktualisieren" klicken → Kurzfazit-Texte + Gesamt-Summary erscheinen

- [ ] **5.3 — Listing Detail View**
  - Form mit allen Feldern, `document_md` gerendert, Grade-Grid
  - `needs_reevaluation`-Badge + "Neu bewerten"-Button (2.2)
  - Status-Management (rejected mit Kommentar)
  - **Test:** Listing in List-View anklicken → Detail-View → Preis editieren → `needs_reevaluation = true` UND `price` in `user_confirmed_fields` → "Neu bewerten" → Badge weg

---

## Layer 6: Chat-UI + Agent-Integration (Chat wird lebendig)

- [ ] **6.1 — Frontend: Chat-UI (Vue, ohne Agent)**
  - `ChatView.vue` — Message-Liste, Input, File-Dropzone
  - Donut-Chart (zeigt bis 6.3 `0%` — `token_count_estimate` wird erst vom Agenten befuellt), "Neue Session"-Button (einfach, ohne Warnung/Disable — kommt in 8.3), Session-Dropdown
  - Nachrichten persistieren, keine Antworten (noch kein Agent)
  - **Test:** Nachricht tippen → Enter → erscheint in Liste → Reload → noch da

- [ ] **6.2 — Provider Interface + Claude-Implementierung**
  - `internal/agent/provider.go` — Interface: `Chat()`, `FormatTools()`, `FormatMessages()`, `SupportsDocumentInput()`
  - `internal/agent/provider/claude/` — Anthropic-Client, SSE-Mapping
  - `ChatEvent`-Stream: `text_delta`, `tool_call_request`, `done`
  - **Model:** Claude 3.5 Sonnet · GPT-4o (Tool-Use-Zuverlaessigkeit ist hier kritisch — kein Platz fuer Halluzinationen im JSON-Format)
  - **Test:** Unit-Test mit Mock-HTTP-Server → Claude-Tool-Call → `ChatEvent` korrekt

- [ ] **6.3 — Agent Loop (ohne Tools, nur Echo)**
  - `internal/agent/loop.go` — `Run()` als Goroutine
  - System-Prompt-Assembly (Preamble + Global Context + Profile Context) — Preamble **ohne Tool-Sektion**, solange der Tool-Katalog leer ist (sonst halluziniert das Modell Tool-Calls als Text)
  - SSE-Streaming der Agent-Antwort ins Frontend
  - `token_count_estimate` aus API-Usage befuellen (Donut ab hier mit Daten)
  - Limits: `max_llm_roundtrips`, `max_tool_calls_per_turn`
  - Noch keine echten Tools — Agent antwortet nur textuell
  - **Model:** Claude 3.5 Sonnet · GPT-4o (erster echter Kontakt mit dem Modell — hier zeigt sich ob der Prompt stimmt)
  - **Test:** Chat-Nachricht senden → Agent antwortet (text only) → Antwort erscheint streamend im Chat

- [ ] **6.4 — First Tools (fetch_url, save_listing, download_image)**
  - Tool-Registry + Dispatch in `loop.go` (Preamble ab hier mit voller Tool-Sektion)
  - `fetch_url` — HTTP GET + Readability-Text-Extraktion
  - `save_listing` — DB-Insert/Update mit `force_overwrite`
  - `download_image` — HTTP GET → `/data/images/...` → `listing_image`-Row
  - Inline-Listing-Card im Chat: `chat_message.listing_id` → Card mit Titel, Preis, Thumbnail, Score; Klick oeffnet Detail-View (5.3)
  - Agent kann: URL → fetch → save → download images → antworten
  - **Model:** Claude 3.5 Sonnet · GPT-4o (erster Multi-Turn-Tool-Use — Modell muss Tool-Ergebnisse korrekt einordnen und weiterfuehren)
  - **Test:** URL in Chat pasten → Agent laedt Seite, speichert Listing, laedt Bilder → Listing sichtbar in List-View (5.2), Bilder in Detail-View, Card im Chat verlinkt

- [ ] **6.5 — Evaluation Tools + Confirm-Flow**
  - `evaluate_listing` Tool (ruft Single-Shot-Call aus 2.2)
  - `propose_criteria` Tool (pending, kein Write)
  - `update_context` Tool (pending, kein Write)
  - Confirm/Reject-Endpoints: `POST .../messages/{mid}/tool-calls/{idx}/confirm`
  - Confirm-Cards im Frontend (Uebernehmen/Verwerfen)
  - **Model:** Claude 3.5 Sonnet · GPT-4o (Pending-Confirmation-Pattern muss zuverlaessig funktionieren — das Modell darf nicht "vergessen" dass es auf Bestaetigung wartet)
  - **Test:** URL in Chat → Agent schlaegt Criteria vor → Confirm-Card erscheint → Uebernehmen klicken → Criteria in DB (sichtbar in 5.1) → evaluate_listing laeuft

- [ ] **6.6 — Remaining Tools**
  - `geocode` Tool (ruft 3.4 auf)
  - `list_profile_listings` Tool
  - `compute_ranking` Tool (Score-Sortierung, kein LLM; Kurzfazit schreibt der Agent im Chat — Persistierung nur via Button aus 5.2)
  - `read_pdf` Tool (PDF-Upload als Claude-Document-Block; PDF nach Ingestion-Turn von Disk loeschen)
  - **Model:** Claude 3.5 Sonnet · GPT-4o (PDF-Input ist Claude-spezifisch — fuer OpenCode braucht es PDF→Text-Extraktion statt nativem Document-Block)
  - **Test:** "Vergleiche mit Kappmuren 2" → Agent nutzt `list_profile_listings`
  - **Test:** "Rangliste aktualisieren" → Agent nutzt `compute_ranking`, schreibt Kurzfazit im Chat

---

## Layer 7: Restliche Views

- [ ] **7.1 — Map View**
  - OpenLayers + OSM-Karte
  - Marker pro Listing mit Koordinaten, farbcodiert nach Status
  - Popup mit Titel, Preis, Score
  - **Test:** Listing mit Koordinaten (manuell in 5.3 oder via Agent aus 6.6) → Map View → Marker sichtbar → Popup klickbar

- [ ] **7.2 — Context Editor**
  - `ContextEditorView.vue` — Markdown-Textarea mit Preview
  - History-Panel (context_revision-Liste) mit Wiederherstellen
  - Global Context ueber Settings-Einstieg, Profile Context ueber Profil-Tab
  - **Test:** Context oeffnen → Text aendern → speichern → History zeigt Revision → Agent (6.3+) beruecksichtigt neuen Context im naechsten Turn

- [ ] **7.3 — Image Gallery im Detail View**
  - Thumbnail-Raster, Kategorie-Badges
  - Stern-Icon fuer Title-Image-Wechsel
  - Loeschen mit Confirmation
  - **Test:** Detail-View oeffnen → Galerie zeigt alle Bilder → Stern klicken → Titelbild gewechselt

---

## Layer 8: Feinschliff

- [ ] **8.1 — Rejection Async Analysis**
  - Status auf `rejected` → async LLM-Call analysiert Kommentar
  - `propose_criteria` / `update_context` Vorschlaege
  - Toast "Vorschlaege im Chat verfuegbar"
  - **Model:** Claude 3.5 Sonnet · GPT-4o-mini (Single-Shot-Analyse, kein Tool-Use — Haiku waere okay, aber hier ist die Qualitaet der Vorschlaege wichtig)
  - **Test:** Listing ablehnen mit Kommentar → Toast → Chat oeffnen → Confirm-Cards sichtbar

- [ ] **8.2 — user_confirmed_fields im Agent-Context**
  - Listing-Context-Block vor jedem Turn
  - `save_listing` skipped gesperrte Felder
  - `force_overwrite` nach User-Bestaetigung
  - **Model:** Claude 3.5 Sonnet · GPT-4o (Modell muss Kontext-Block mit gesperrten Feldern korrekt lesen und befolgen)
  - **Test:** Preis manuell editieren (5.3, setzt Flag via 2.1) → Agent: "nicht ueberschrieben" → User: "ueberschreib" → Preis aktualisiert

- [ ] **8.3 — "Neue Session" mit Warnung + Disable**
  - Button deaktiviert waehrend Turn laeuft
  - Dialog warnt bei offenen Confirm-Cards
  - Alte Sessions (read-only, `ended_at` gesetzt) als Dropdown einsehbar
  - **Test:** Turn laeuft → Button grau → Confirm-Card offen → "Neue Session" klicken → Warnung erscheint

- [ ] **8.4 — Stop-Button + Fehlende-Antwort-Handling**
  - Stop setzt Cancel-Flag, laufender Tool-Call laeuft zu Ende
  - "Antwort fehlt — erneut senden?" unter User-Message ohne Agent-Antwort
  - **Test:** Turn laeuft → Stop klicken → Turn bricht ab → User-Message ohne Antwort → Button erscheint

---

## Abhaengigkeiten

```
0.1 ──→ 0.2 ──→ 1.1 ──→ 1.2
                    │
                    ├──→ 1.3
                    │
                    ├──→ 2.1 ──→ 2.2 ──→ 2.3
                    │         │
                    │         └──→ 3.1 ──→ 3.2
                    ├──→ 3.3
                    ├──→ 3.4
                    │
                    └──→ 4.1 ──→ 4.2 ──→ 5.1 ──→ 5.2 ──→ 5.3
                                                  │
                                                  └──→ 6.1 ──→ 6.2 ──→ 6.3 ──→ 6.4 ──→ 6.5 ──→ 6.6
                                                                                            │
                                                                                            ├──→ 7.1 ──→ 7.2 ──→ 7.3
                                                                                            │
                                                                                            └──→ 8.x (parallel)
```

Layer 7 kann parallel zu Layer 6 laufen — die Views nutzen die REST-APIs aus Layer 1-3 und die UI-Patterns aus Layer 5. 7.2 (Context Editor) kann bei Bedarf vorgezogen werden, um das Agent-Verhalten ab 6.3 im UI statt per curl zu steuern.
