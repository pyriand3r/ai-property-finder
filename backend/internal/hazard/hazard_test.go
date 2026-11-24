package hazard

import (
	"encoding/json"
	"maps"
	"propertyfinder/internal/domain"
	"slices"
	"strings"
	"testing"
)

// validFile is a small, valid dataset: two sources (wildfire, earthquake),
// one region with both ratings and one with a wildfire rating only. Each test
// breaks exactly one thing in it.
func validFile() datasetFile {
	return datasetFile{
		Country: "PT",
		Sources: []domain.HazardSource{
			{
				ID: "pt-fire", HazardType: domain.HazardWildfire, Title: "Fire map", Publisher: "ICNF",
				URL: "https://example.test/fire", License: "CC BY 4.0", DatasetVersion: "2020",
				RetrievedOn: "2026-11-24", Method: "mean class",
			},
			{
				ID: "pt-quake", HazardType: domain.HazardEarthquake, Title: "Seismic zones", Publisher: "IPQ",
				URL: "https://example.test/quake", License: "n/a", DatasetVersion: "2010",
				RetrievedOn: "2026-11-24", Method: "agR bands",
			},
		},
		Regions: []regionFile{
			{
				Key: "PT-0502", Name: "Castelo Branco", ParentName: "Castelo Branco",
				Ratings: map[domain.HazardType]int{domain.HazardEarthquake: 3, domain.HazardWildfire: 4},
			},
			{
				Key: "PT-0705", Name: "Évora", ParentName: "Évora",
				Ratings: map[domain.HazardType]int{domain.HazardWildfire: 1},
			},
		},
	}
}

func parseFile(t *testing.T, modify func(*datasetFile)) (Dataset, error) {
	t.Helper()
	file := validFile()
	if modify != nil {
		modify(&file)
	}
	data, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	return Parse(data)
}

func TestParse_ValidDataset_ExpandsRatingsInHazardOrderWithTheirTypesSource(t *testing.T) {
	dataset, err := parseFile(t, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Wildfire comes before earthquake in domain.HazardTypes, whatever the
	// JSON map order was; the source id is the one of the rating's hazard type.
	want := []domain.HazardRating{
		{RegionKey: "PT-0502", HazardType: domain.HazardWildfire, Rating: 4, SourceID: "pt-fire"},
		{RegionKey: "PT-0502", HazardType: domain.HazardEarthquake, Rating: 3, SourceID: "pt-quake"},
		{RegionKey: "PT-0705", HazardType: domain.HazardWildfire, Rating: 1, SourceID: "pt-fire"},
	}
	if !slices.Equal(dataset.Ratings, want) {
		t.Errorf("ratings = %+v, want %+v", dataset.Ratings, want)
	}
	if len(dataset.Regions) != 2 || dataset.Regions[1].Country != "PT" || dataset.Regions[1].NameSearch != "evora" {
		t.Errorf("regions = %+v, want Évora of PT searchable as %q", dataset.Regions, "evora")
	}
	if len(dataset.Sources) != 2 {
		t.Errorf("sources = %d, want 2", len(dataset.Sources))
	}
}

// TestParse_InvalidDataset_NamesTheProblem: every rule below protects the
// database or a user from a bad hand-edited dataset file. Asserting the
// message (not just "some error") proves the intended rule fired.
func TestParse_InvalidDataset_NamesTheProblem(t *testing.T) {
	cases := []struct {
		name    string
		modify  func(*datasetFile)
		wantErr string
	}{
		{"rating below the scale", func(f *datasetFile) { f.Regions[0].Ratings[domain.HazardWildfire] = 0 }, "outside 1-5"},
		{"rating above the scale", func(f *datasetFile) { f.Regions[0].Ratings[domain.HazardWildfire] = 6 }, "outside 1-5"},
		{"unknown hazard type in a region", func(f *datasetFile) { f.Regions[0].Ratings["tsunami"] = 3 }, `unknown hazard type "tsunami"`},
		{"rating whose hazard has no source", func(f *datasetFile) { f.Regions[0].Ratings[domain.HazardFlood] = 2 }, "no flood source"},
		{"region without any rating", func(f *datasetFile) { f.Regions[1].Ratings = map[domain.HazardType]int{} }, "has no ratings"},
		{"region without a name", func(f *datasetFile) { f.Regions[0].Name = " " }, "has no name"},
		{"duplicate region key", func(f *datasetFile) { f.Regions[1].Key = f.Regions[0].Key }, "appears twice"},
		{"region key of another country", func(f *datasetFile) { f.Regions[0].Key = "ES-0502" }, "not a PT-<code> key"},
		{"lower-case region key", func(f *datasetFile) { f.Regions[0].Key = "pt-0502" }, "not a PT-<code> key"},
		{"country that is no ISO code", func(f *datasetFile) { f.Country = "PRT" }, "ISO 3166-1"},
		{"no regions", func(f *datasetFile) { f.Regions = nil }, "no regions"},
		{"no sources", func(f *datasetFile) { f.Sources = nil }, "no sources"},
		{"source that no rating uses", func(f *datasetFile) {
			f.Sources = append(f.Sources, domain.HazardSource{
				ID: "pt-slide", HazardType: domain.HazardLandslide, Title: "t", Publisher: "p",
				URL: "https://example.test/s", License: "l", DatasetVersion: "v", RetrievedOn: "2026-11-24", Method: "m",
			})
		}, `source "pt-slide" is not used`},
		{"two sources for one hazard type", func(f *datasetFile) {
			second := f.Sources[0]
			second.ID = "pt-fire-2"
			f.Sources = append(f.Sources, second)
		}, "two sources"},
		{"duplicate source id", func(f *datasetFile) { f.Sources[1].ID = f.Sources[0].ID }, "appears twice"},
		{"source with unknown hazard type", func(f *datasetFile) { f.Sources[0].HazardType = "tsunami" }, "unknown hazard type"},
		{"source without method", func(f *datasetFile) { f.Sources[0].Method = "  " }, "method is empty"},
		{"source url that is no http(s) url", func(f *datasetFile) { f.Sources[0].URL = "ftp://example.test/x" }, "not an http(s) URL"},
		{"source retrieval date in another format", func(f *datasetFile) { f.Sources[0].RetrievedOn = "24.11.2026" }, "YYYY-MM-DD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseFile(t, tc.modify)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestParse_MalformedJSON_IsRejected(t *testing.T) {
	valid, err := json.Marshal(validFile())
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		data    string
		wantErr string
	}{
		"empty input":      {"", "decode"},
		"not json":         {"country: PT", "decode"},
		"unknown field":    {`{"country":"PT","regionz":[]}`, "unknown field"},
		"data after JSON":  {string(valid) + `{}`, "unexpected data"},
		"rating as string": {strings.Replace(string(valid), `"wildfire":4`, `"wildfire":"4"`, 1), "decode"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, parseError := Parse([]byte(tc.data)); parseError == nil || !strings.Contains(parseError.Error(), tc.wantErr) {
				t.Errorf("err = %v, want one containing %q", parseError, tc.wantErr)
			}
		})
	}
}

func TestNormalizeName_IgnoresCaseAccentsHyphensAndSpacing(t *testing.T) {
	cases := map[string]string{
		"Évora":                "evora",
		"ÉVORA":                "evora",
		"São João da Madeira":  "sao joao da madeira",
		"Albergaria-a-Velha":   "albergaria a velha",
		"  Castelo   Branco  ": "castelo branco",
		"Câmara de Lobos":      "camara de lobos",
		"Vila Nova de Foz Côa": "vila nova de foz coa",
		"":                     "",
	}
	for input, want := range cases {
		if got := NormalizeName(input); got != want {
			t.Errorf("NormalizeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func portugal(t *testing.T) Dataset {
	t.Helper()
	datasets, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, dataset := range datasets {
		if dataset.Country == "PT" {
			return dataset
		}
	}
	t.Fatal("no PT dataset embedded")
	return Dataset{}
}

// TestLoad_EmbeddedPortugal_CoversEveryMunicipalityAsDocumented guards the
// data that ships: a bad dataset file stops the backend at boot, so it must
// fail here first. The coverage rule is the one in spec 01-13: every
// mainland municipality (district codes 01-18) has all four hazards, the
// islands (Madeira 31/32, Azores 41-49) have earthquake only, because no
// source covers them for the other three.
func TestLoad_EmbeddedPortugal_CoversEveryMunicipalityAsDocumented(t *testing.T) {
	dataset := portugal(t)
	if len(dataset.Regions) != 308 {
		t.Errorf("%d regions, want the 308 municipalities of CAOP 2025", len(dataset.Regions))
	}

	hazardsOf := make(map[string][]domain.HazardType)
	for _, rating := range dataset.Ratings {
		hazardsOf[rating.RegionKey] = append(hazardsOf[rating.RegionKey], rating.HazardType)
	}
	for _, region := range dataset.Regions {
		district := region.Key[len("PT-") : len("PT-")+2]
		want := []domain.HazardType{domain.HazardEarthquake}
		if district >= "01" && district <= "18" {
			want = domain.HazardTypes()
		}
		if got := hazardsOf[region.Key]; !slices.Equal(got, want) {
			t.Errorf("%s %s has ratings for %v, want %v", region.Key, region.Name, got, want)
		}
	}
}

// TestLoad_EmbeddedPortugal_KeepsKnownAnswers pins a few municipalities to
// the values the sources give (spec 01-13 explains each rule): if a dataset
// is regenerated and one of these moves, the change was either intended — then
// update the table and the spec — or a join or threshold went wrong.
func TestLoad_EmbeddedPortugal_KeepsKnownAnswers(t *testing.T) {
	type ratings = map[domain.HazardType]int
	want := map[string]ratings{
		// Lisboa: seismic zones 1.3 / 2.3 (agR 1.7), small vegetated share, Tejo/Trancão ARPSI.
		"PT-1106": {domain.HazardWildfire: 2, domain.HazardFlood: 2, domain.HazardEarthquake: 3, domain.HazardLandslide: 3},
		// Faro: seismic zone 1.2 (agR 2.0), coastal ARPSI.
		"PT-0805": {domain.HazardWildfire: 2, domain.HazardFlood: 3, domain.HazardEarthquake: 4, domain.HazardLandslide: 3},
		// Castelo Branco: no ARPSI, seismic zones 1.6 / 2.4.
		"PT-0502": {domain.HazardWildfire: 3, domain.HazardFlood: 1, domain.HazardEarthquake: 2, domain.HazardLandslide: 1},
		// Ponta Delgada (Azores): Type 2 zone 2.1, the highest agR (2.5).
		"PT-4203": {domain.HazardEarthquake: 5},
		// Funchal (Madeira): Type 1 zone 1.6, the lowest agR (0.35).
		"PT-3103": {domain.HazardEarthquake: 1},
	}
	got := make(map[string]ratings)
	for _, rating := range portugal(t).Ratings {
		if _, known := want[rating.RegionKey]; known {
			if got[rating.RegionKey] == nil {
				got[rating.RegionKey] = ratings{}
			}
			got[rating.RegionKey][rating.HazardType] = rating.Rating
		}
	}
	for key, wantRatings := range want {
		if !maps.Equal(got[key], wantRatings) {
			t.Errorf("%s ratings = %v, want %v", key, got[key], wantRatings)
		}
	}
}
