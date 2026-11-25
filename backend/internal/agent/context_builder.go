package agent

import (
	"fmt"
	"propertyfinder/internal/domain"
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
Du hast Zugriff auf Tools (fetch_url, save_listing, evaluate_listing,
list_profile_listings, compute_ranking, geocode, …). Nutze sie proaktiv.
Sage nie „das kann ich nicht abrufen" – ruf es ab. Wenn ein Tool
fehlschlägt, melde den Fehler im Chat und schlage eine Alternative vor.

## Ablauf bei einem neuen Inserat (URL)
1. fetch_url auf die Hauptseite
2. Folge relevanten sekundären und tertiären Links mit fetch_url, z.B.
   Portal → Maklerseite → Exposé- oder Grundriss-PDF. Portal-Regeln geben
   die wichtigen Links vor; folge auch ohne Regel klar erkennbaren Links zu
   weiteren Inseratsdetails oder Dokumenten. PDFs werden direkt mitgelesen.
3. Vor dem Speichern: list_profile_listings prüfen, ob source_url schon
   existiert (Duplikat) – falls ja, im Chat nachfragen statt stillschweigend
   ein zweites Inserat anzulegen
4. download_image für alle relevanten Fotos, Kategorien vergeben
5. save_listing mit allen Feldern + document_md (Originaltext-Auszug,
   deutsche Übersetzung, Bewertungstabelle, Vor-/Nachteile)
6. evaluate_listing
7. geocode falls Koordinaten fehlen
8. Ergebnis im Chat zusammenfassen

## PDF-Upload
Ein vom Nutzer hochgeladenes PDF ist bereits als natives Dokument Teil
dieser Nachricht – kein Tool-Call nötig, lies es direkt. Ablauf identisch
zum URL-Fall ab Schritt 4, aber ohne sekundäre Links (das PDF ist die
Quelle). Fehlen Fotos im PDF und der Nutzer nennt eine Quell-URL, kannst
du zusätzlich download_image von dort aufrufen.

## Vergleich und Rangliste
- list_profile_listings für Vergleichsfragen ("vergleiche mit X") oder um
  vor save_listing ein Duplikat zu erkennen
- compute_ranking liefert die Score-Rangliste (deterministisch, kein
  LLM-Call in diesem Tool selbst) – das Kurzfazit pro Platz formulierst
  du im Chat-Text, nicht das Tool

## Naturgefahren-Quellen
Die App kennt Naturgefahren-Daten (Waldbrand, Hochwasser, Erdbeben, Erdrutsch)
nur für Länder und Gefahren, die in ihrem Quellenkatalog stehen. Liegt ein
Inserat in einem Gebiet ohne solche Daten und kennst du aus einer gelesenen
Seite eine öffentliche Quelle, rufe suggest_hazard_source auf. Der Vorschlag
ändert nichts und lädt nichts herunter. Sag dem Nutzer, dass er ihn im
Globalen Kontext freigeben oder ablehnen kann. Erfinde keine URLs. Meldet das
Tool „schon im Katalog" oder „schon vorgeschlagen", frage nicht erneut.

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
// Context, then — if listingContextMD is non-empty — the current Listing's
// confirmed-fields block (ADR 0016, Layer 8.2). hasTools gates the Tool-
// dependent preamble sections (see systemPreambleTools); maxRoundtrips
// fills the "Grenzen" section so the model knows its own budget for this
// turn. Empty blocks are omitted rather than sent blank.
func BuildSystemPrompt(hasTools bool, maxRoundtrips int, globalContextMD, profileContextMD, listingContextMD string) []ContentBlock {
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
	if strings.TrimSpace(listingContextMD) != "" {
		blocks = append(blocks, ContentBlock{Type: ContentBlockText, Text: listingContextMD})
	}
	return blocks
}

// confirmableListingFields is every field name save_listing's
// force_overwrite skip logic protects (agent/tools.go applyListingFields)
// — the same set BuildListingContextBlock renders, in the same order, so
// the block the Agent reads and the fields save_listing actually protects
// never drift apart.
var confirmableListingFields = []string{
	"title", "source_url", "price", "price_currency", "property_size_m2",
	"plot_size_m2", "address", "region", "country", "latitude", "longitude",
	"description", "seller_name", "seller_phone", "seller_email", "seller_notes",
}

// BuildListingContextBlock renders the "Aktuelles Inserat" block (ADR
// 0016) — the current value of every manually confirmed field, so the
// Agent knows upfront (before attempting save_listing) which fields are
// locked and what they're currently set to, rather than only finding out
// reactively from a skipped_fields tool result. Empty when
// listing.UserConfirmedFields is empty — callers should skip appending it
// in that case (BuildSystemPrompt already omits blank blocks either way).
func BuildListingContextBlock(listing domain.Listing) string {
	if len(listing.UserConfirmedFields) == 0 {
		return ""
	}
	confirmed := make(map[string]bool, len(listing.UserConfirmedFields))
	for _, field := range listing.UserConfirmedFields {
		confirmed[field] = true
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Aktuelles Inserat (%s)\n", listing.Title)
	b.WriteString("Manuell bestätigte Felder — nur nach expliziter Zustimmung überschreiben:\n")
	for _, field := range confirmableListingFields {
		if !confirmed[field] {
			continue
		}
		if value := listingFieldValue(listing, field); value != "" {
			fmt.Fprintf(&b, "- %s: %s\n", field, value)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// listingFieldValue renders one confirmable field's current value in
// plain text, "" if unset. Field names/set matches
// confirmableListingFields exactly.
func listingFieldValue(listing domain.Listing, field string) string {
	switch field {
	case "title":
		return listing.Title
	case "source_url":
		return derefStr(listing.SourceURL)
	case "price":
		if listing.Price == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprintf("%.0f %s", *listing.Price, derefStr(listing.PriceCurrency)))
	case "price_currency":
		return derefStr(listing.PriceCurrency)
	case "property_size_m2":
		return derefFloat(listing.PropertySizeM2)
	case "plot_size_m2":
		return derefFloat(listing.PlotSizeM2)
	case "address":
		return derefStr(listing.Address)
	case "region":
		return derefStr(listing.Region)
	case "country":
		return derefStr(listing.Country)
	case "latitude":
		return derefFloat(listing.Latitude)
	case "longitude":
		return derefFloat(listing.Longitude)
	case "description":
		return derefStr(listing.Description)
	case "seller_name":
		return derefStr(listing.SellerName)
	case "seller_phone":
		return derefStr(listing.SellerPhone)
	case "seller_email":
		return derefStr(listing.SellerEmail)
	case "seller_notes":
		return derefStr(listing.SellerNotes)
	default:
		return ""
	}
}

func derefStr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func derefFloat(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *v)
}
