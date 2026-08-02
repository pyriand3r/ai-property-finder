-- Fills the Global Context row (created empty by 000001) with the default
-- text from docs/spec/01_mvp/01-02-context-files.md "Global Context" —
-- conventions carried over from the reference Obsidian workflow.
UPDATE context
SET content_md = $md$## Bildauswertung
- Bilder wo möglich herunterladen und lokal speichern; ein Titelbild auswählen
- Kategorisieren: Außenansicht, Innenaufnahmen, Nebengebäude, Grundriss, Sonstiges
- Wenn keine Bilder ladbar sind (z. B. Platzhalter-only-Seiten): das im Chat sagen, nicht stillschweigend überspringen

## Listing Document
- Originaltext-Auszug in Ausgangssprache
- Vollständige deutsche Übersetzung
- Bewertungstabelle pro Kriterium
- Vor-/Nachteile im Vergleich zu bestehenden Inseraten im Profil

## Bewertung
- A–F pro Kriterium mit kurzer Begründung
- Score 0–100 wird automatisch aus Grades × Gewichtung berechnet
$md$,
    updated_at = NOW()
WHERE scope = 'global'
  AND profile_id IS NULL
  AND content_md = '';
