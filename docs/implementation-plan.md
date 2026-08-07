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

- [x] **2.2 — evaluate_listing (Single-Shot-LLM-Call)** — verified with one live Claude API call (`claude-sonnet-5`): real Grade A + German reasoning, `overall_score` computed, `listing_criterion_grade` row persisted
  - `POST /api/listings/{id}/evaluate` — kein Agent, kein Tool-Use
  - Prompt: Criterion-Liste + Listing-Daten → Grades mit Reasoning + Score
  - Provider-Interface wie dev-property-finder: `Evaluate(ctx, criteria, listing) -> ([]Grade, score, error)`
  - `needs_reevaluation = false` bei Erfolg
  - **Model:** Claude 3.5 Sonnet · GPT-4o-mini (Single-Shot, strukturiertes JSON — Haiku wuerde reichen, aber Sonnet ist sicherer bei Edge-Cases)
  - **Test:** Criterion anlegen (1.3), Listing anlegen (2.1), `curl -X POST .../api/listings/{id}/evaluate` → DB: `overall_score > 0`, `listing_criterion_grade`-Rows existieren

- [x] **2.3 — needs_reevaluation + Bulk-Trigger**
  - `GET /api/profiles/{id}/listings?needs_reevaluation=true` → nur stale Listings
  - `POST /api/profiles/{id}/listings/reevaluate-all` → sequenzielle Goroutine, SSE-Stream (Task-SSE-Plumbing, wird in 6.3 fuer Chat-SSE wiederverwendet)
  - **Test:** Criterion-Beschreibung aendern → `needs_reevaluation = true` auf allen Listings → `curl .../reevaluate-all` → SSE-Stream zeigt Fortschritt → Scores aktualisiert
  - **Test:** Criterion-Weight aendern → Scores sofort neu per SQL, Flag bleibt `false` (kein LLM-Call)

---

## Layer 3: File-Handling (Bilder, PDF, Geocoding)

- [x] **3.1 — Image Upload + Storage + Serving**
  - `/media/` → `http.FileServer(http.Dir("/data"))` im chi-Router
  - `POST /api/listings/{id}/images` — Multipart-Upload, UUID-rename
  - `GET /api/listings/{id}/images` — Liste aller `listing_image`-Rows
  - `DELETE /api/listings/{id}/images/{image_id}` — Datei + DB-Row loeschen
  - **Test:** `curl -F "file=@photo.jpg" .../api/listings/{id}/images` → `201` → `curl .../media/images/listings/{id}/{uuid}.jpg` → Bild sichtbar

- [x] **3.2 — Title Image setzen/aendern**
  - `PATCH /api/listings/{id}` mit `{"title_image_id":"..."}`
  - Erstes Image-Upload: `title_image_id` automatisch setzen
  - **Test:** Zwei Bilder hochladen → `title_image_id` auf erstes → per PATCH auf zweites wechseln

- [x] **3.3 — PDF Upload**
  - `POST /api/upload/pdf` — Multipart, max 32 MB, speichert Temp-Datei
  - **Test:** `curl -F "file=@listing.pdf" .../api/upload/pdf` → `201` mit Pfad

- [x] **3.4 — Geocoding (Nominatim)**
  - `internal/geocode/nominatim.go` — HTTP GET
  - `GET /api/geocode?address=...&country=...` → `{"lat","lon","display_name"}`
  - Rate-Limiting: Mutex + Last-Call-Timestamp
  - **Test:** `curl ".../api/geocode?address=Kappmuren+Malmo"` → Koordinaten

---

## Layer 4: Chat-Basis + Frontend-Skelett

- [x] **4.1 — Chat Session + Message Persistence**
  - Session-CRUD, Message-CRUD (nur User-Nachrichten, noch kein Agent)
  - `POST /api/profiles/{id}/chat/sessions/{sid}/messages` speichert nur
  - `POST /api/profiles/{id}/chat/sessions` legt neue Session an: alte Session bekommt `ended_at` gesetzt (read-only), neue Row ist aktiv
  - **Test:** Nachricht senden → persistiert → Reload → noch da
  - **Test:** Zweite Session anlegen → alte hat `ended_at` gesetzt, beide in DB

- [x] **4.2 — Frontend-Skelett + Profil-Liste (Vue)**
  - Scaffolding: Vite + TypeScript + Vue Router + Naive UI; Go embedt `dist/` (Single-Binary), Dev-Proxy auf `:8080`
  - `npm add -D oxlint oxfmt` — Configs (`.oxlintrc.json`, `.oxfmtrc.json`) liegen bereits im Repo-Root; npm-Scripts `lint`/`format`/`typecheck` anlegen. **Kein** ESLint/Prettier.
  - `ProfileListView.vue` — Liste, Archiv, Create/Rename/Archive
  - Klick auf Profil → Profil-Shell mit Tab-Strip (Platzhalter-Tabs)
  - **Test:** Browser → Profil anlegen → anklicken → Profil-Shell erscheint

---

## Layer 5: Manuelle Views (Form-Fallback komplett nutzbar)

Macht die App vor dem Agenten als manuellen Finder mit AI-Scoring nutzbar. Nutzt nur die fertigen REST-APIs aus Layer 1-3.

- [x] **5.1 — Criterion Editor**
  - Liste aller Criteria mit Weight-Slider, Add/Edit/Delete
  - Weight-Aenderung → Scores aktualisieren sofort (SQL-Recompute), kein Banner
  - Andere Aenderungen → Banner "N Inserate neu bewerten" + "Alle neu bewerten"-Button (2.3)
  - **Test:** Weight aendern → Scores neu, kein Banner → Beschreibung aendern → Banner erscheint → "Alle neu bewerten" klicken → SSE-Fortschritt

- [x] **5.2 — List View + Ranking View**
  - Sortierbare/filterbare Tabelle (Score, Preis, Groesse, Datum; Filter Status/Score/Preis)
  - Ranking: Score-Sortierung + Kurzfazit + Gesamt-Summary (`search_profile.ranking_summary`)
  - "Rangliste aktualisieren"-Button: isolierter Single-Shot-LLM-Call (kein Agent, nutzt Provider-Seam aus 2.2), speichert `listing.kurzfazit` + `search_profile.ranking_summary`
  - **Model:** Claude 3.5 Sonnet · GPT-4o-mini (Text-Generierung, kein Tool-Use — Haiku reicht fuer Kurztexte)
  - **Test:** Mehrere Listings → List View zeigt alle → Ranking View sortiert → "Rangliste aktualisieren" klicken → Kurzfazit-Texte + Gesamt-Summary erscheinen

- [x] **5.3 — Listing Detail View**
  - Form mit allen Feldern, `document_md` gerendert, Grade-Grid
  - `needs_reevaluation`-Badge + "Neu bewerten"-Button (2.2)
  - Status-Management (rejected mit Kommentar)
  - **Test:** Listing in List-View anklicken → Detail-View → Preis editieren → `needs_reevaluation = true` UND `price` in `user_confirmed_fields` → "Neu bewerten" → Badge weg

---

## Layer 6: Chat-UI + Agent-Integration (Chat wird lebendig)

- [x] **6.1 — Frontend: Chat-UI (Vue, ohne Agent)**
  - `ChatView.vue` — Message-Liste, Input, File-Dropzone
  - Donut-Chart (zeigt bis 6.3 `0%` — `token_count_estimate` wird erst vom Agenten befuellt), "Neue Session"-Button (einfach, ohne Warnung/Disable — kommt in 8.3), Session-Dropdown
  - Nachrichten persistieren, keine Antworten (noch kein Agent)
  - **Test:** Nachricht tippen → Enter → erscheint in Liste → Reload → noch da

- [x] **6.2 — Provider Interface + Claude-Implementierung**
  - `internal/agent/provider.go` — Interface: `Chat()`, `FormatTools()`, `FormatMessages()`, `SupportsDocumentInput()`
  - `internal/agent/provider/claude/` — Anthropic-Client, SSE-Mapping
  - `ChatEvent`-Stream: `text_delta`, `tool_call_request`, `done`
  - **Model:** Claude 3.5 Sonnet · GPT-4o (Tool-Use-Zuverlaessigkeit ist hier kritisch — kein Platz fuer Halluzinationen im JSON-Format)
  - **Test:** Unit-Test mit Mock-HTTP-Server → Claude-Tool-Call → `ChatEvent` korrekt

- [x] **6.3 — Agent Loop (ohne Tools, nur Echo)**
  - `internal/agent/loop.go` — `Run()` als Goroutine
  - System-Prompt-Assembly (Preamble + Global Context + Profile Context) — Preamble **ohne Tool-Sektion**, solange der Tool-Katalog leer ist (sonst halluziniert das Modell Tool-Calls als Text)
  - SSE-Streaming der Agent-Antwort ins Frontend
  - `token_count_estimate` aus API-Usage befuellen (Donut ab hier mit Daten)
  - Limits: `max_llm_roundtrips`, `max_tool_calls_per_turn`
  - Noch keine echten Tools — Agent antwortet nur textuell
  - **Model:** Claude 3.5 Sonnet · GPT-4o (erster echter Kontakt mit dem Modell — hier zeigt sich ob der Prompt stimmt)
  - **Test:** Chat-Nachricht senden → Agent antwortet (text only) → Antwort erscheint streamend im Chat

- [x] **6.4 — First Tools (fetch_url, save_listing, download_image)**
  - Tool-Registry + Dispatch in `loop.go` (Preamble ab hier mit voller Tool-Sektion)
  - `fetch_url` — HTTP GET + Readability-Text-Extraktion
  - `save_listing` — DB-Insert/Update mit `force_overwrite`
  - `download_image` — HTTP GET → `/data/images/...` → `listing_image`-Row
  - Inline-Listing-Card im Chat: `chat_message.listing_id` → Card mit Titel, Preis, Thumbnail, Score; Klick oeffnet Detail-View (5.3)
  - Agent kann: URL → fetch → save → download images → antworten
  - **Model:** Claude 3.5 Sonnet · GPT-4o (erster Multi-Turn-Tool-Use — Modell muss Tool-Ergebnisse korrekt einordnen und weiterfuehren)
  - **Test:** URL in Chat pasten → Agent laedt Seite, speichert Listing, laedt Bilder → Listing sichtbar in List-View (5.2), Bilder in Detail-View, Card im Chat verlinkt

- [x] **6.5 — Evaluation Tools + Confirm-Flow**
  - `evaluate_listing` Tool (ruft Single-Shot-Call aus 2.2)
  - `propose_criteria` Tool (pending, kein Write)
  - `update_context` Tool (pending, kein Write)
  - Confirm/Reject-Endpoints: `POST .../messages/{mid}/tool-calls/{idx}/confirm`
  - Confirm-Cards im Frontend (Uebernehmen/Verwerfen)
  - **Model:** Claude 3.5 Sonnet · GPT-4o (Pending-Confirmation-Pattern muss zuverlaessig funktionieren — das Modell darf nicht "vergessen" dass es auf Bestaetigung wartet)
  - **Test:** URL in Chat → Agent schlaegt Criteria vor → Confirm-Card erscheint → Uebernehmen klicken → Criteria in DB (sichtbar in 5.1) → evaluate_listing laeuft

- [x] **6.6 — Remaining Tools**
  - `geocode` Tool (ruft 3.4 auf)
  - `list_profile_listings` Tool
  - `compute_ranking` Tool (Score-Sortierung, kein LLM; Kurzfazit schreibt der Agent im Chat — Persistierung nur via Button aus 5.2)
  - `read_pdf` Tool (PDF-Upload als Claude-Document-Block; PDF nach Ingestion-Turn von Disk loeschen)
  - **Model:** Claude 3.5 Sonnet · GPT-4o (PDF-Input ist Claude-spezifisch — fuer OpenCode braucht es PDF→Text-Extraktion statt nativem Document-Block)
  - **Test:** "Vergleiche mit Kappmuren 2" → Agent nutzt `list_profile_listings`
  - **Test:** "Rangliste aktualisieren" → Agent nutzt `compute_ranking`, schreibt Kurzfazit im Chat

---

## Review-Fixes (Zwischenschritt vor Layer 7)

Nach einem manuellen Review-Durchgang durch die laufende App (`docs/review.md`) gefundene Bugs und UX-Lücken. Keiner der Punkte war in einem späteren Layer eingeplant (geprüft gegen den gesamten Plan) — alles hier ist ungeplante Nacharbeit, kein Vorziehen künftiger Arbeit. Eigene Nummerierung (`RF.x`), damit Layer 7/8 nicht umnummeriert werden müssen.

- [ ] **RF.1 — Chat-Zuverlässigkeit: Confirm-Card-Status + Budget-Limit-Fortsetzung**
  - Confirm-Card-Status (`confirmed`/`rejected`) wird in `chat_message.tool_calls` JSONB persistiert statt nur lokaler Frontend-State — überlebt Tab-Wechsel/Reload. Confirm/Reject werden idempotent (409 bei erneuter Aktion). Siehe ADR 0017.
  - `budgetExhausted`-Zweig in `loop.go` persistiert nur Text (Roundtrip-Text + Limit-Hinweis), keine nicht-dispatchten `tool_use`-Blöcke mehr — verhindert ungültige Replay-History, die "weitermachen" bisher wirkungslos machte.
  - Spec: [[../docs/spec/01_mvp/01-09-chat-reliability-fixes]]
  - **Test:** Vorschlag bestätigen → Tab wechseln/reload → Card zeigt weiterhin "Übernommen". `llm.max_llm_roundtrips` testweise niedrig setzen, Limit auslösen, "weitermachen" antworten → Agent macht tatsächlich weiter.

- [ ] **RF.2 — Token-Donut Lesbarkeit**
  - `TokenDonut.vue`: Ring-Größe/Stroke-Width angepasst, damit der Füllstand auf einen Blick erkennbar ist.
  - Spec: [[../docs/spec/01_mvp/01-01-chat-interface#Layout]]
  - **Test:** Visueller Check in `npm run dev`.

- [ ] **RF.3 — Listing List/Detail View Überarbeitung**
  - Master-Detail-Split (Liste links, Detail rechts) in der "Liste"-Tab statt Navigation auf separate Seite; `/listings/:id`-Route bleibt für Deep-Links (Chat-Card) erhalten.
  - Titelbild-Thumbnails in Liste und Detail-Ansicht.
  - Optionale, persistente Pro-Kriterium-Score-Spalten in der Liste (localStorage je Profil).
  - Bewertungstabelle an den Anfang der Detail-Ansicht, Status prominent im Header statt im Formularraster.
  - Explizites Bearbeiten/Speichern/Abbrechen statt Auto-Save pro Feld — bündelt Änderungen in einem PATCH-Call.
  - Quell-URL als klickbarer Link im Anzeige-Modus.
  - Spec: [[../docs/spec/01_mvp/01-10-listing-views-overhaul]] (ersetzt Teile von 01-05 § List View und 01-07 § Editing Behaviour)
  - **Test:** siehe Verification-Abschnitt der Spec.

- [ ] **RF.4 — Listing-Notizen (neues Feature)**
  - Volles CRUD auf der bereits existierenden `listing_note`-Tabelle (Migration ergänzt `updated_at`): Repository, Service, API (`/api/listings/{id}/notes`, `/api/notes/{id}`), Frontend-Cards mit "Neue Notiz"/Bearbeiten/Löschen.
  - Ausdrücklich getrennt von `seller_notes` (Feld zum Verkäufer, bleibt unverändert).
  - Spec: [[../docs/spec/01_mvp/01-11-listing-notes]]
  - **Test:** curl gegen neue Endpoints, dann manueller Klick-Durchlauf.

- [ ] **RF.5 — Chat-Feedback-Fixes**
  - Turn-In-Progress-Indikator: `useChat.ts`-Flag ab erfolgreichem POST bis zum ersten Live-Signal (`chat.text_delta`/`chat.tool_progress`/`chat.message`/`chat.turn_error`) für den Turn; `ChatMessageList.vue` zeigt währenddessen einen animierten Platzhalter, auch bei stillen Lücken zwischen Tool-Calls innerhalb eines Turns.
  - Unicode-Escape-Bug (Vorschlagstext zeigt `\uXXXX` statt Umlauten): bei frischem Live-Test nicht reproduzierbar — Spec dokumentiert offene Diagnoseschritte statt eines Fixes.
  - Spec: [[../docs/spec/01_mvp/01-12-chat-feedback-fixes]]
  - **Test:** Nachricht senden → Indikator erscheint sofort, verschwindet bei erstem Text/Tool-Progress; Multi-Roundtrip-Turn auslösen → Indikator erscheint erneut bei Lücke zwischen Tool-Calls. Unicode-Bug bleibt offen bis reproduziert.

---

## Layer 7: Restliche Views

- [x] **7.1 — Map View**
  - OpenLayers + OSM-Karte, neuer "Karte"-Tab in `ProfileShellView.vue`, lazy geladen (`defineAsyncComponent`)
  - Master-Detail-Split wie List View: Karte links, `ListingDetailPanel` rechts (`ListingSplitPane.vue`, geteilt mit der Inserate-Liste)
  - Marker pro Listing mit Koordinaten, farbcodiert nach Status (`src/utils/listingStatus.ts`)
  - Marker-Klick: wählt Listing für die rechte Spalte aus **und** öffnet ein kleines Popup am Marker mit Titel, Preis, Score, 100x100-Thumbnail
  - Spec: [[../docs/spec/01_mvp/01-06-map-view]]
  - **Test:** Listing mit Koordinaten (manuell in 5.3 oder via Agent aus 6.6) → Karte-Tab → Marker sichtbar, farbcodiert → Marker klicken → Popup mit Thumbnail erscheint, rechte Spalte zeigt Detail-Ansicht

- [x] **7.2 — Context Editor**
  - `ContextEditor.vue` — Markdown-Textarea (Bearbeiten → Speichern/Abbrechen), Preview via marked+DOMPurify im Anzeige-Modus
  - History-Panel (context_revision-Liste) mit Wiederherstellen (Confirm-Dialog, neuer Revision-Snapshot vor dem Zurücksetzen)
  - Backend: `GET /api/context/global/history`, `POST .../revisions/{id}/restore` (global + profile) — Restore prüft, dass die Revision zum Ziel-Context gehört
  - Global Context über Link auf der Profilliste (`/context`), Profile Context über Profil-Tab
  - Spec: [[../docs/spec/01_mvp/01-02-context-files]]
  - **Test:** Context öffnen → Text ändern → speichern → History zeigt Revision → Wiederherstellen setzt alten Text zurück (curl-verifiziert, inkl. 400 bei Revision aus falschem Context) → Agent liest Context frisch pro Turn (agent_service.go, unverändert seit Layer 6.3)

- [x] **7.3 — Image Gallery im Detail View**
  - `ListingGallery.vue` — Thumbnail-Raster, Kategorie-Badges (Außenansicht/Innenaufnahme/Grundriss/Nebengebäude/Sonstiges)
  - Stern-Icon fuer Title-Image-Wechsel (PATCH `title_image_id`, bereits ADR-0012-konform, kein neuer Endpoint noetig)
  - Loeschen mit Confirmation (`useDialog`, DELETE `/api/listings/{id}/images/{imageId}`, bereits vorhandener Endpoint)
  - Spec: [[../docs/adr/0012-image-storage-filesystem-media]]
  - **Test:** Detail-View oeffnen → Galerie zeigt alle Bilder mit Kategorie-Badge → Stern auf anderem Bild klicken → Titelbild + Header-Thumbnail wechseln (live verifiziert) → Testbild hochgeladen (curl) → Löschen-Icon → Confirm-Dialog → Bild entfernt, Titelbild unveraendert (live verifiziert)

---

## Layer 8: Feinschliff

- [x] **8.1 — Rejection Async Analysis**
  - Status auf `rejected` → async LLM-Call analysiert Kommentar (`RejectionAnalysisService`, single-shot wie `EvaluationService`/`RankingService`, kein Tool-Use)
  - `propose_criteria` / `update_context` Vorschlaege landen als synthetische `chat_message` (role=assistant, `tool_calls` im selben Format wie ein echter Agent-Turn) in der aktiven Session — bestehende Confirm-Card-UI/Endpoints unveraendert wiederverwendet
  - Toast "Rejection verarbeitet — Vorschlaege im Chat verfuegbar." (`useRejectionAnalysisToast.ts`, SSE-Event `listing.rejection_analyzed`)
  - **Model:** Claude (ADR 0006 — einziger MVP-Provider; Single-Shot-Analyse, kein Tool-Use)
  - Spec: [[../docs/spec/01_mvp/01-04-criteria-and-scoring#Rejection Feedback, Reimagined]]
  - **Test:** Listing per PATCH auf `rejected` mit Kommentar gesetzt (curl + UI) → `update_context`-Vorschlag korrekt erkannt (Makler-Meide-Regel) → Confirm-Card in Chat sichtbar (inkl. Listing-Card) → Übernehmen funktioniert unveraendert → Toast im Browser bestaetigt. golangci-lint/go test clean (inkl. neuer `parseRejectionAnalysis`-Tests).

- [x] **8.2 — user_confirmed_fields im Agent-Context**
  - Listing-Context-Block vor jedem Turn (`BuildListingContextBlock`, `context_builder.go`) — "aktuelles Inserat" ermittelt aus der juengsten `listing_id` in der Session-History (`currentListingID`, `agent_service.go`)
  - `save_listing` skipped gesperrte Felder (bereits seit ADR 0016 vorhanden)
  - `force_overwrite` nach User-Bestaetigung (bereits seit ADR 0016 vorhanden)
  - **Model:** Claude (ADR 0006 — einziger MVP-Provider)
  - Spec: [[../docs/adr/0016-user-confirmed-fields-agent-context]]
  - **Test:** Live gegen echte Claude API (neue Chat-Session wegen kaputter alter Session, siehe Buglist): Preis manuell auf 315.000 SEK gesetzt → direkte Nachfrage "welche Felder gesperrt?" beantwortet der Agent ungefragt korrekt mit exaktem Wert (kein Tool-Call, Block kam nachweislich im System-Prompt an) → Region manuell gesperrt, vage Bitte um Änderung → Agent lehnt ab, fragt nach ("nicht überschrieben") → DB bestätigt unveraendert → "Ja, überschreib." → Agent aktualisiert per `force_overwrite`, Feld korrekt aus `user_confirmed_fields` entfernt, DB bestätigt Änderung. golangci-lint/go vet/go test clean (inkl. neuer `context_builder_test.go`).

- [x] **8.3 — "Neue Session" mit Warnung + Disable**
  - Button deaktiviert waehrend Turn laeuft (`turnInProgress` in `useChat.ts`, gesetzt zwischen erfolgreichem POST und `chat.message`/`chat.turn_error`)
  - Dialog warnt bei offenen Confirm-Cards (`hasPendingConfirmCards`, geteilte Helper in `src/utils/chat.ts`)
  - Alte Sessions (read-only, `ended_at` gesetzt) als Dropdown einsehbar (bereits seit Layer 6.1 vorhanden)
  - Bugfix waehrend Live-Verifikation gefunden: `ChatConfirmCard.vue`s `status` war rein lokaler State — Uebernehmen/Verwerfen aktualisierte nie `useChat.ts`s `messages`, wodurch `hasPendingConfirmCards` nach Aufloesung einer Card veraltet blieb. Fix: `ChatConfirmCard.vue` emitted `resolved`, `ChatMessageList.vue` reicht es mit Message-ID + Call-Index als `callResolved` weiter, `useChat.ts` bekommt `markToolCallResolved` zum In-Place-Patchen von `messages`.
  - **Test:** `turnInProgress` per MutationObserver zeitstempelbelegt bestaetigt (disabled:true bei Turnstart, disabled:false 8.04s spaeter bei Turnende, immun gegen eigene Tool-Roundtrip-Latenz). Confirm-Card ("Testkriterium 8.3b") im Browser verworfen → sofort "Neue Session" geklickt → Dialog zeigt korrekt den einfachen Text ohne Pending-Warnung (vorher: Warnung blieb faelschlich stehen). oxlint/oxfmt/vue-tsc/`npm run build` clean, Docker-Rebuild.

- [x] **8.4 — Stop-Button + Fehlende-Antwort-Handling**
  - Backend: `AgentService` haelt pro Session einen `context.CancelFunc` (`turns map[uuid.UUID]context.CancelFunc`), synchron registriert bevor `SendMessage` zurueckkehrt (kein Race mit einem sofortigen Stop-Klick). `StopTurn` validiert Session-Profil-Zugehoerigkeit und ruft `cancel()`. Neuer Endpunkt `POST .../chat/sessions/{sessionId}/stop`.
  - `agent.Loop.Run` prueft `ctx.Err()` vor jedem Roundtrip; bei Abbruch (vor oder waehrend des Streamings, erkannt via `errors.Is(streamErr, context.Canceled)`) persistiert `stopped()` eine Text-only-Abschluss-Roundtrip ("[Abgebrochen durch Nutzer.]") — nie mit `tool_use`-Bloecken fuer nicht dispatchte Calls (gleiches Prinzip wie beim Budget-Limit, verhindert dangling `tool_use` beim naechsten Turn-Replay).
  - `dispatchAll` nutzt `context.WithoutCancel(ctx)` fuer den eigentlichen Tool-Aufruf — ein laufender Tool-Call (z. B. `fetch_url`, `save_listing`) laeuft immer zu Ende, nur die naechste LLM-Anfrage wird nicht mehr gestellt.
  - Zwei Bugs waehrend Live-Verifikation gefunden und gefixt: (1) `persistRoundtrip`/`UpdateTokenCount` nutzten den bereits abgebrochenen `ctx` fuer den DB-Write, wodurch der Abschluss-Roundtrip nie persistiert wurde und `chat.message` nie feuerte (frontend blieb dauerhaft im `turnInProgress`-Zustand haengen) — gefixt mit einem von `ctx` entkoppelten `persistCtx := context.WithoutCancel(ctx)` fuer alle Schreiboperationen. (2) Ein abgebrochener Turn ohne gemessenen Usage (`tokenCount == 0`, da nie ein `ChatEventDone` ankam) ueberschrieb `token_count_estimate` faelschlich auf 0 — gefixt mit einem `tokenCount > 0`-Guard vor dem `UpdateTokenCount`-Aufruf.
  - Frontend: `useChat.ts` `stop()` ruft den neuen Endpunkt auf (404 durch Race mit natuerlichem Turn-Ende wird verschluckt); `missingReply` erkennt eine letzte User-Message ohne jede Folge-Nachricht (nur nach Server-Neustart moeglich, da ein Stop immer seine eigene Abschluss-Nachricht persistiert); `retryLastMessage` sendet den gleichen Inhalt erneut (keine History-Bearbeitung). `ChatView.vue` zeigt einen "Stop"-Button neben "Neue Session" waehrend `turnInProgress`; `ChatMessageList.vue` zeigt "Antwort fehlt — erneut senden?" unter der letzten Nachricht wenn `missingReply`.
  - **Test:** `go test ./internal/agent/...` deckt drei Cancel-Pfade ab (zwischen Roundtrips, mitten im Stream, Tool-Call laeuft trotz Cancel zu Ende — `dispatchCtx.Err()` bleibt `nil` obwohl der aeussere `ctx` bereits abgebrochen ist). Live gegen echte Claude API: Stop-Button waehrend eines laufenden Turns geklickt → Button verschwindet, "Neue Session" wieder aktiv, "[Abgebrochen durch Nutzer.]"-Bubble erscheint sofort im Chat (curl+Browser). `token_count_estimate` vor/nach einem abgebrochenen Turn verglichen (25546 → 25546, nicht 0) — Regression aus Bug (2) bestaetigt behoben. Server-Neustart mitten in einem laufenden Turn simuliert (`docker compose restart backend` direkt nach dem POST) → letzte Nachricht bleibt `role=user` ohne Folge-Nachricht → "Antwort fehlt — erneut senden?" erscheint im Browser → Klick sendet erfolgreich neu und die Antwort kommt vollstaendig durch. golangci-lint/go vet/go test clean.

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
                                                                                            └──→ RF.1 ──→ RF.2 ──→ RF.3 ──→ RF.4 ──→ RF.5
                                                                                                                                       │
                                                                                                                                       ├──→ 7.1 ──→ 7.2 ──→ 7.3
                                                                                                                                       │
                                                                                                                                       └──→ 8.x (parallel)
```

Layer 7 kann parallel zu Layer 6 laufen — die Views nutzen die REST-APIs aus Layer 1-3 und die UI-Patterns aus Layer 5. 7.2 (Context Editor) kann bei Bedarf vorgezogen werden, um das Agent-Verhalten ab 6.3 im UI statt per curl zu steuern.
