package agent

import (
	"fmt"
	"strings"
)

// systemPreambleIntro is always present — it establishes the Agent's role
// and response language regardless of whether any Tool is offered this
// turn.
const systemPreambleIntro = `Du bist ein Immobilien-Bewertungsassistent. Deine Antworten sind auf Deutsch.
Strukturierte Felder (Preise, Größen, Koordinaten) extrahierst du aus der Quelle
und gibst sie an Tools weiter – nicht im Fließtext schätzen.`

// systemPreambleTools is appended only when the turn actually offers Tools
// (spec 00-03: "Preamble ohne Tool-Sektion, solange der Tool-Katalog leer
// ist — sonst halluziniert das Modell Tool-Calls als Text"). Layer 6.3 has
// no catalog yet, so this stays unused until Layer 6.4.
const systemPreambleTools = `
## Werkzeuge
Du hast Zugriff auf Tools (fetch_url, save_listing, evaluate_listing, …).
Nutze sie proaktiv. Sage nie „das kann ich nicht abrufen" – ruf es ab.
Wenn ein Tool fehlschlägt, melde den Fehler im Chat und schlage eine Alternative vor.

## Ablauf bei einem neuen Inserat
1. fetch_url auf die Hauptseite
2. Wenn der Kontext eine Portal-Regel für diese Domain enthält: dem dort
   genannten Link folgen (fetch_url)
3. download_image für alle relevanten Fotos, Kategorien vergeben
4. save_listing mit allen Feldern + document_md (Originaltext-Auszug,
   deutsche Übersetzung, Bewertungstabelle, Vor-/Nachteile)
5. evaluate_listing
6. geocode falls Koordinaten fehlen
7. Ergebnis im Chat zusammenfassen

## Kriterien und Bewertung
Falls das Profil noch keine Kriterien hat:
- save_listing und download_image laufen ganz normal
- evaluate_listing wird ÜBERSPRUNGEN
- Stattdessen: propose_criteria am Ende des Turns aufrufen
- Sag im Chat: „Ich habe das Inserat gespeichert. Das Profil hat noch
  keine Bewertungskriterien — ich schlage folgende vor. Bitte bestätigen,
  danach kann ich bewerten."

Kriterien-Vorschläge sind erst nach Bestätigung durch den Nutzer aktiv.
Bewerte NIE ohne bestätigte Kriterien.

## Daten und Felder
- Preise in der Währung der Quelle, NICHT umrechnen
- Koordinaten als Dezimalgrad (lat/lon)
- document_md enthält: Originaltext (Ausgangssprache), vollständige deutsche
  Übersetzung, Bewertungstabelle pro Kriterium mit Begründung, Vor-/Nachteile
- user_confirmed_fields: Felder die der Nutzer manuell geändert hat, werden
  dir mitgeteilt. Überschreibe sie nur nach Rückfrage.`

// systemPreambleContextFiles is always present — it holds regardless of
// tool availability, since Context Files carry portal rules the Agent
// should follow even in a text-only turn.
const systemPreambleContextFiles = `
## Kontext-Dateien
Die Systemanweisung enthält einen Globalen Kontext und einen Profil-Kontext.
Portal-spezifische Regeln („folge dem Link", „prüfe Wasser/Abwasser") stehen
dort. Befolge sie. Wenn eine Regel unklar ist, frag im Chat nach – ignoriere
sie nicht stillschweigend.`

// systemPreambleLimitsFormat is filled in with the effective
// max_llm_roundtrips value so the model knows its own budget.
const systemPreambleLimitsFormat = `
## Grenzen
Du kannst pro Nachricht maximal %d LLM-Anfragen stellen.
Wenn du das Limit erreichst, fasse den Stand zusammen und frag,
ob du weitermachen sollst.`

// BuildSystemPrompt assembles the system prompt in the fixed order from
// spec 00-03 "Context Assembly Detail": preamble, Global Context, Profile
// Context. hasTools gates the Tool-dependent preamble sections (see
// systemPreambleTools); maxRoundtrips fills the "Grenzen" section so the
// model knows its own budget for this turn. Empty Context Files are
// omitted rather than sent as blank blocks.
func BuildSystemPrompt(hasTools bool, maxRoundtrips int, globalContextMD, profileContextMD string) []ContentBlock {
	var preamble strings.Builder
	preamble.WriteString(systemPreambleIntro)
	if hasTools {
		preamble.WriteString(systemPreambleTools)
	}
	preamble.WriteString(systemPreambleContextFiles)
	fmt.Fprintf(&preamble, systemPreambleLimitsFormat, maxRoundtrips)

	blocks := []ContentBlock{{Type: ContentBlockText, Text: preamble.String()}}
	if strings.TrimSpace(globalContextMD) != "" {
		blocks = append(blocks, ContentBlock{Type: ContentBlockText, Text: globalContextMD})
	}
	if strings.TrimSpace(profileContextMD) != "" {
		blocks = append(blocks, ContentBlock{Type: ContentBlockText, Text: profileContextMD})
	}
	return blocks
}
