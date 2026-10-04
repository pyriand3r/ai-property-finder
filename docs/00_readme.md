---
type: project
created: 2026-07-28
title: Property Finder OBS-Style — Projektbeschreibung
project: property-finder-obs-style
status: active
started: 2026-07-28
due:
goal: Web-App-Spec für einen chat-gesteuerten Immobilien-Finder, der den bewährten Obsidian-Workflow (immobilienplan-schweden-portugal-oesterreich/schweden) ablöst
tags:
  - property-finder-obs-style
  - beschreibung
  - konzept
---
## Beschreibung

Nachfolgeprojekt von [[../dev-property-finder/00_readme|dev-property-finder]]. Die alte Spec war komplett durchgeplant (12 ADRs, volles MVP), bildet aber den **falschen Workflow** ab: formularbasiertes CRUD mit fest kodierten Scraper-Modulen pro Portal. Der Workflow, der in der Praxis tatsächlich funktioniert und produktiv genutzt wird, läuft in Obsidian: eine KI im Chat bekommt eine URL (oder ein PDF), liest das Inserat, folgt portalspezifischen Links, extrahiert Bilder, schreibt ein strukturiertes Dokument, bewertet es transparent nach Kriterien und pflegt Rangliste + Karte.

Diese App-Idee überträgt genau diesen Workflow in eine Web-App — mit Chat-Fenster als Herzstück statt Formularen, und mit persistentem, editierbarem Kontext pro Such-Profil (Pendant zu einer profil-eigenen AGENT.md) sowie einem globalen Kontext (Pendant zur vault-weiten AGENT.md).

Das transparente A–F-Bewertungsschema mit gewichteten Kriterien aus der alten Spec bleibt — das Modell war nie das Problem, nur die Art, wie Daten hineinkommen.

## Grundidee

- **Chat statt Formular** als primäre Eingabe: der User gibt der KI im Chat Anweisungen ("bewerte dieses Inserat", "folge dem Makler-Link", "vergleiche mit den anderen"), keine Multi-Schritt-Formulare
- **Agentische Inserats-Verarbeitung**: KI bekommt URL oder PDF, ruft Tools auf (Seite laden, Bilder extrahieren, Makler-Link folgen), schreibt strukturierten Datensatz + lesbaren Volltext (Original + Übersetzung + Bewertung + Vor-/Nachteile — wie die heutigen Obsidian-Dateien)
- **Zwei Kontext-Ebenen, analog AGENT.md**:
  - Globaler Kontext: app-weite Grundregeln, teils vorbelegt (Bildextraktion, Tabellenformat, Sternebewertung), vom User erweiterbar
  - Profil-Kontext: pro Such-Profil eigene Anweisungen (z. B. "bei schwedischen Objekten immer Wasser/Abwasser prüfen und Booli-Makler-Link folgen")
- **Transparentes Bewertungsschema**: AI-definierte Kriterien, A–F-Grade pro Kriterium, gewichteter Gesamt-Score — wie in [[../dev-property-finder/spec.md|dev-property-finder/spec.md]], aber Kriterien direkt editierbar, nicht nur über Fließtext-Reparse
- **Liste, Ranking, Karte**: wie im Obsidian-Dashboard — sortierte Liste, KI-erzeugte Rangliste mit Kurzfazit, OSM-Karte mit Koordinaten
- **Volle Nachvollziehbarkeit**: jedes KI-befüllte Feld bleibt manuell editierbar; Änderungen lösen Re-Evaluation aus

## Warum der alte Ansatz falsch war

| Alt (dev-property-finder) | Neu (dieses Projekt) |
|---|---|
| Formular-CRUD pro Feld | Chat als primäres Interface |
| Ein Fließtext pro Profil → KI parsed Kriterien einmalig | Laufender Dialog, Kriterien entstehen/wachsen iterativ, direkt editierbar |
| Fest kodierter Scraper + HTML-Reduction-Pipeline pro Portal (ADR 0007/0008) | Agent mit Web-Fetch-Tool, portalspezifische Regeln als Text im Kontext, kein Custom-Code pro Portal |
| Kein Umgang mit PDF-Fallback bei Anti-Bot-Blockade | PDF-Upload nativ unterstützt (Claude API kann PDFs direkt lesen) |
| Kein persistenter "Wie soll ich arbeiten"-Kontext | Globaler + profil-spezifischer Kontext, AGENT.md-artig, persistent und editierbar |

Details: [[spec.md]], [[CONTEXT.md]], [[user-stories.md]].

## Ziel

Eine belastbare Spec für eine Web-App, die den Obsidian-Workflow aus `immobilienplan-schweden-portugal-oesterreich/schweden` 1:1 abbildet und dabei das transparente Bewertungsmodell der alten dev-property-finder-Spec wiederverwendet.

## Endkriterium

Spec ist vollständig genug, um Phase-1-Entwicklung (Chat-Interface + Kontext-Dateien + manuelle/URL-Ingestion + Bewertung + Liste) zu starten, ohne Rückfragen zur Grundarchitektur.

## Notizen

- Referenz-Workflow: [[../immobilienplan-schweden-portugal-oesterreich/schweden/00_readme|immobilienplan-schweden-portugal-oesterreich/schweden]] — dort stehen die "Arbeitsanweisungen für zukünftige Sessions", die 1:1 die Rolle des Profil-Kontexts übernehmen sollen
- Alte Spec nicht löschen — Datenmodell (Kriterien, Grades, Gewichtung, Score-Formel) wird übernommen, nur die Ingestion- und Interaktions-Schicht ändert sich grundlegend
- Testkatalog bewusst noch nicht angelegt — erst nach Validierung der Grundidee (siehe Nächste Schritte)
- ADRs für Chat/Context/Agent-Architektur angelegt: [[docs/adr/README|docs/adr/]] (6 ADRs, eigene Nummerierung, ADR 0003/0004 lösen dev-property-finder ADR 0007/0008/0001 ab)
- 2026-07-29: Wechsel des POC-Providers zu OpenCode wurde erwogen und verworfen — Machbarkeit (verlässlicher Multi-Turn-Tool-Use, natives PDF-Input) spricht für Claude; es bleibt bei [[docs/adr/0006-single-provider-for-agent-mvp|ADR 0006]]
- Deployment auf einen entfernten Docker-Host (Image als Datei, ohne Registry): [[deployment]]

## Nächste Schritte

- [ ] Spec-Review mit Fokus auf Tool-Definitionen für den Agenten (spec/00_infrastructure_setup/00-03-llm-agent-integration.md)
- [x] Entscheiden: eigenständiger Chat-Agent-Loop selbst bauen vs. Anthropic Agent SDK/Tool-Runner nutzen — entschieden: eigener, provider-unabhängiger Loop, siehe [[docs/adr/0007-agent-loop-provider-independent]]
- [x] Prüfen, ob PDF-Direktupload an Claude API den pymupdf-Workaround aus dem Obsidian-Workflow vollständig ersetzt — entschieden, siehe [[docs/adr/0005-pdf-native-document-input]]
- [x] ADRs für die zentralen Architekturentscheidungen angelegt (docs/adr/)
- [ ] Testkatalog nach Vorbild [[../dev-property-finder/tests/index|dev-property-finder/tests/index]] aufsetzen
- [ ] LM Studio / OpenCode Provider-Implementierung nach MVP-Validierung neu bewerten (siehe [[docs/adr/0006-single-provider-for-agent-mvp]])
