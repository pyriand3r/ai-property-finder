package hazard

import (
	"encoding/json"
	"propertyfinder/internal/domain"
	"strings"
	"testing"
)

// catalogDatasets is a one-source Portugal dataset the catalog cases build on.
func catalogDatasets() []Dataset {
	return []Dataset{{
		Country: "PT",
		Sources: []domain.HazardSource{{
			ID: "pt-fire", HazardType: domain.HazardWildfire, Title: "Fire map", Publisher: "ICNF",
			URL: "https://example.test/fire", License: "open", DatasetVersion: "v1", RetrievedOn: "2026-11-24", Method: "m",
		}},
	}}
}

func validCatalog() catalogFile {
	return catalogFile{Country: "PT", Entries: []domain.HazardCatalogEntry{
		{ID: "pt-fire-icnf", HazardType: domain.HazardWildfire, Scope: "Portugal", Status: domain.HazardCatalogBundled, SourceID: "pt-fire"},
		{
			ID: "pt-flood-x", HazardType: domain.HazardFlood, Scope: "Azoren", Status: domain.HazardCatalogCandidate,
			Title: "Flood", Publisher: "Region", URL: "https://example.test/flood", Notes: "license unknown",
		},
	}}
}

func parseCatalogFile(t *testing.T, modify func(*catalogFile)) ([]domain.HazardCatalogEntry, error) {
	t.Helper()
	file := validCatalog()
	if modify != nil {
		modify(&file)
	}
	data, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	return ParseCatalog(data, catalogDatasets())
}

// TestParseCatalog_BundledEntry_TakesItsFieldsFromTheDatasetSource: the
// catalog states a shipped source once, in the dataset, so the two cannot
// disagree.
func TestParseCatalog_BundledEntry_TakesItsFieldsFromTheDatasetSource(t *testing.T) {
	entries, err := parseCatalogFile(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	bundled := entries[0]
	if bundled.Country != "PT" || bundled.Title != "Fire map" || bundled.Publisher != "ICNF" ||
		bundled.URL != "https://example.test/fire" || bundled.License != "open" {
		t.Errorf("bundled entry not resolved from its source: %+v", bundled)
	}
	if entries[1].Status != domain.HazardCatalogCandidate || entries[1].Title != "Flood" {
		t.Errorf("candidate changed: %+v", entries[1])
	}
}

func TestParseCatalog_InvalidCatalog_NamesTheProblem(t *testing.T) {
	cases := []struct {
		name    string
		modify  func(*catalogFile)
		wantErr string
	}{
		{"country that is no ISO code", func(f *catalogFile) { f.Country = "PRT" }, "ISO 3166-1"},
		{"id without the country prefix", func(f *catalogFile) { f.Entries[1].ID = "flood-x" }, `must start with "pt-"`},
		{"id that is only the prefix", func(f *catalogFile) { f.Entries[1].ID = "pt-" }, `must start with "pt-"`},
		{"duplicate id", func(f *catalogFile) { f.Entries[1].ID = f.Entries[0].ID }, "appears twice"},
		{"unknown hazard type", func(f *catalogFile) { f.Entries[1].HazardType = "tsunami" }, "unknown hazard type"},
		{"empty scope", func(f *catalogFile) { f.Entries[1].Scope = " " }, "scope is empty"},
		{"unknown status", func(f *catalogFile) { f.Entries[1].Status = "planned" }, "unknown status"},
		{"bundled entry that restates the url", func(f *catalogFile) { f.Entries[0].URL = "https://example.test/other" }, "must not state"},
		{"bundled entry with an unknown source", func(f *catalogFile) { f.Entries[0].SourceID = "pt-nope" }, "not a source of the PT dataset"},
		{"bundled entry with the wrong hazard type", func(f *catalogFile) { f.Entries[0].HazardType = domain.HazardFlood }, "is a wildfire source"},
		{"dataset source without a bundled entry", func(f *catalogFile) { f.Entries = f.Entries[1:] }, `"pt-fire" has no bundled catalog entry`},
		{"two entries for one source", func(f *catalogFile) {
			second := f.Entries[0]
			second.ID = "pt-fire-2"
			f.Entries = append(f.Entries, second)
		}, "already named by entry"},
		{"candidate that names a source", func(f *catalogFile) { f.Entries[1].SourceID = "pt-fire" }, "no dataset source"},
		{"candidate without a title", func(f *catalogFile) { f.Entries[1].Title = "" }, "title is empty"},
		{"candidate without notes", func(f *catalogFile) { f.Entries[1].Notes = "" }, "notes is empty"},
		{"candidate url that is no http(s) url", func(f *catalogFile) { f.Entries[1].URL = "ftp://example.test/x" }, "not an http(s) URL"},
		{"same hazard type and url twice", func(f *catalogFile) {
			second := f.Entries[1]
			second.ID = "pt-flood-y"
			second.URL = "http://www.example.test/flood/"
			f.Entries = append(f.Entries, second)
		}, "same hazard type and URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseCatalogFile(t, tc.modify)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestParseCatalog_MalformedJSON_IsRejected(t *testing.T) {
	for name, data := range map[string]string{
		"unknown field": `{"country":"PT","entriez":[]}`,
		"trailing data": `{"country":"PT","entries":[]} {}`,
	} {
		if _, err := ParseCatalog([]byte(data), catalogDatasets()); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestSourceURLKey_IgnoresSchemeWwwFragmentAndTrailingSlash(t *testing.T) {
	same := []string{
		"https://hora.gv.at/",
		"http://www.HORA.gv.at",
		" https://hora.gv.at/#karte ",
	}
	for _, raw := range same {
		if got := SourceURLKey(raw); got != "hora.gv.at" {
			t.Errorf("SourceURLKey(%q) = %q, want hora.gv.at", raw, got)
		}
	}
	if a, b := SourceURLKey("https://x.test/a?id=1"), SourceURLKey("https://x.test/a?id=2"); a == b {
		t.Errorf("different queries must stay different sources, both %q", a)
	}
	for _, raw := range []string{"", "ftp://x.test", "not a url", "https://"} {
		if got := SourceURLKey(raw); got != "" {
			t.Errorf("SourceURLKey(%q) = %q, want empty", raw, got)
		}
	}
}

// TestLoadCatalog_Embedded_ListsEveryShippedSourceAndCandidates: the files that
// ship pass validation, every dataset source is cataloged, and the three
// launch countries each have a catalog.
func TestLoadCatalog_Embedded_ListsEveryShippedSourceAndCandidates(t *testing.T) {
	entries, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	bundled := map[string]int{}
	countries := map[string]bool{}
	for _, entry := range entries {
		countries[entry.Country] = true
		if entry.Status == domain.HazardCatalogBundled {
			bundled[entry.Country]++
		}
	}
	for _, country := range []string{"PT", "SE", "AT"} {
		if !countries[country] {
			t.Errorf("no catalog entry for %s", country)
		}
	}
	if bundled["PT"] != 4 {
		t.Errorf("PT bundled entries = %d, want 4 (wildfire, flood, earthquake, landslide)", bundled["PT"])
	}
	if bundled["SE"] != 0 || bundled["AT"] != 0 {
		t.Errorf("SE/AT have no dataset yet, but bundled entries are %d/%d", bundled["SE"], bundled["AT"])
	}
}
