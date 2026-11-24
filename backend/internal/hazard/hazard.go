// Package hazard loads the static, region-level natural-hazard datasets that
// ship inside the binary (docs/adr/0025, docs/spec/01_mvp/01-13-hazard-ratings.md).
//
// It touches neither the network nor the database: Load parses and validates
// the embedded dataset files, and service.HazardService syncs the result into
// the hazard_* tables at boot. A dataset file that fails validation stops the
// backend from starting, so bad data never reaches a user.
package hazard

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"propertyfinder/internal/domain"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// datasetFiles holds one JSON file per country. Updating a dataset means
// replacing its file; the next boot syncs the change into the database.
//
//go:embed datasets/*.json
var datasetFiles embed.FS

var (
	countryPattern   = regexp.MustCompile(`^[A-Z]{2}$`)
	regionKeyPattern = regexp.MustCompile(`^[A-Z]{2}-[0-9A-Z]+$`)
)

// ValidCountry reports whether s is an upper-case ISO 3166-1 alpha-2 code.
func ValidCountry(s string) bool { return countryPattern.MatchString(s) }

// ValidRegionKey reports whether s looks like a region key:
// "<country code>-<national code>", for example "PT-1106".
func ValidRegionKey(s string) bool { return regionKeyPattern.MatchString(s) }

// Dataset is one country's validated hazard data, ready to sync. Every rating
// names the source of its hazard type, and every region has at least one
// rating.
type Dataset struct {
	Country string
	Sources []domain.HazardSource
	Regions []domain.HazardRegion
	Ratings []domain.HazardRating
}

// datasetFile is the on-disk format of a dataset: the sources once, and one
// entry per region with its ratings by hazard type. A hazard type missing
// from a region's ratings means "no usable source data for this region".
// Each hazard type has exactly one source in the file, so a rating does not
// repeat its source id.
type datasetFile struct {
	Country string                `json:"country"`
	Sources []domain.HazardSource `json:"sources"`
	Regions []regionFile          `json:"regions"`
}

type regionFile struct {
	Key        string                    `json:"key"`
	Name       string                    `json:"name"`
	ParentName string                    `json:"parent_name"`
	Ratings    map[domain.HazardType]int `json:"ratings"`
}

// Load parses and validates every embedded dataset file.
func Load() ([]Dataset, error) {
	names, err := fs.Glob(datasetFiles, "datasets/*.json")
	if err != nil {
		return nil, fmt.Errorf("list hazard dataset files: %w", err)
	}
	datasets := make([]Dataset, 0, len(names))
	fileByCountry := make(map[string]string, len(names))
	for _, name := range names {
		data, readError := datasetFiles.ReadFile(name)
		if readError != nil {
			return nil, fmt.Errorf("read %s: %w", name, readError)
		}
		dataset, parseError := Parse(data)
		if parseError != nil {
			return nil, fmt.Errorf("%s: %w", name, parseError)
		}
		if other, seen := fileByCountry[dataset.Country]; seen {
			return nil, fmt.Errorf("%s and %s both hold the dataset for %s", other, name, dataset.Country)
		}
		fileByCountry[dataset.Country] = name
		datasets = append(datasets, dataset)
	}
	return datasets, nil
}

// Parse decodes one dataset file and checks everything the database would
// otherwise reject late (or not at all): unknown fields and hazard types,
// ratings outside 1-5, malformed or duplicate region keys, regions of
// another country, sources that are missing, doubled or unused.
func Parse(data []byte) (Dataset, error) {
	var file datasetFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return Dataset{}, fmt.Errorf("decode: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Dataset{}, errors.New("decode: unexpected data after the dataset object")
	}
	if !ValidCountry(file.Country) {
		return Dataset{}, fmt.Errorf("country %q is not an ISO 3166-1 alpha-2 code", file.Country)
	}

	sourceByType, err := indexSources(file.Sources)
	if err != nil {
		return Dataset{}, err
	}
	dataset := Dataset{Country: file.Country, Sources: file.Sources}
	used := make(map[domain.HazardType]bool, len(sourceByType))
	seenKeys := make(map[string]bool, len(file.Regions))
	for _, entry := range file.Regions {
		region, ratings, regionError := buildRegion(file.Country, entry, sourceByType, seenKeys)
		if regionError != nil {
			return Dataset{}, regionError
		}
		dataset.Regions = append(dataset.Regions, region)
		for _, rating := range ratings {
			used[rating.HazardType] = true
		}
		dataset.Ratings = append(dataset.Ratings, ratings...)
	}
	if len(dataset.Regions) == 0 {
		return Dataset{}, errors.New("dataset has no regions")
	}
	for _, hazardType := range domain.HazardTypes() {
		if source, hasSource := sourceByType[hazardType]; hasSource && !used[hazardType] {
			return Dataset{}, fmt.Errorf("source %q is not used by any rating", source.ID)
		}
	}
	return dataset, nil
}

// indexSources validates every source and returns them by hazard type. A
// dataset has exactly one source per hazard type it covers.
func indexSources(sources []domain.HazardSource) (map[domain.HazardType]domain.HazardSource, error) {
	byType := make(map[domain.HazardType]domain.HazardSource, len(sources))
	ids := make(map[string]bool, len(sources))
	for _, source := range sources {
		if err := validateSource(source); err != nil {
			return nil, fmt.Errorf("source %q: %w", source.ID, err)
		}
		if ids[source.ID] {
			return nil, fmt.Errorf("source id %q appears twice", source.ID)
		}
		ids[source.ID] = true
		if other, dup := byType[source.HazardType]; dup {
			return nil, fmt.Errorf("hazard type %s has two sources (%q and %q); a dataset has one per type", source.HazardType, other.ID, source.ID)
		}
		byType[source.HazardType] = source
	}
	if len(byType) == 0 {
		return nil, errors.New("dataset has no sources")
	}
	return byType, nil
}

func validateSource(source domain.HazardSource) error {
	required := []struct{ field, value string }{
		{"id", source.ID},
		{"title", source.Title},
		{"publisher", source.Publisher},
		{"url", source.URL},
		{"license", source.License},
		{"dataset_version", source.DatasetVersion},
		{"method", source.Method},
	}
	for _, entry := range required {
		if strings.TrimSpace(entry.value) == "" {
			return fmt.Errorf("%s is empty", entry.field)
		}
	}
	if !source.HazardType.Valid() {
		return fmt.Errorf("unknown hazard type %q", source.HazardType)
	}
	parsed, err := url.Parse(source.URL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return fmt.Errorf("url %q is not an http(s) URL", source.URL)
	}
	if _, err = time.Parse(time.DateOnly, source.RetrievedOn); err != nil {
		return fmt.Errorf("retrieved_on %q is not a YYYY-MM-DD date", source.RetrievedOn)
	}
	return nil
}

// buildRegion validates one region entry and expands its ratings in
// domain.HazardTypes order, so the result does not depend on map order.
func buildRegion(
	country string,
	region regionFile,
	sourceByType map[domain.HazardType]domain.HazardSource,
	seenKeys map[string]bool,
) (domain.HazardRegion, []domain.HazardRating, error) {
	if !ValidRegionKey(region.Key) || !strings.HasPrefix(region.Key, country+"-") {
		return domain.HazardRegion{}, nil, fmt.Errorf("region key %q is not a %s-<code> key", region.Key, country)
	}
	if seenKeys[region.Key] {
		return domain.HazardRegion{}, nil, fmt.Errorf("region key %q appears twice", region.Key)
	}
	seenKeys[region.Key] = true
	if strings.TrimSpace(region.Name) == "" {
		return domain.HazardRegion{}, nil, fmt.Errorf("region %q has no name", region.Key)
	}
	for hazardType := range region.Ratings {
		if !hazardType.Valid() {
			return domain.HazardRegion{}, nil, fmt.Errorf("region %q: unknown hazard type %q", region.Key, hazardType)
		}
	}

	var ratings []domain.HazardRating
	for _, hazardType := range domain.HazardTypes() {
		rating, rated := region.Ratings[hazardType]
		if !rated {
			continue
		}
		if rating < domain.HazardRatingMin || rating > domain.HazardRatingMax {
			return domain.HazardRegion{}, nil, fmt.Errorf("region %q: %s rating %d is outside %d-%d",
				region.Key, hazardType, rating, domain.HazardRatingMin, domain.HazardRatingMax)
		}
		source, hasSource := sourceByType[hazardType]
		if !hasSource {
			return domain.HazardRegion{}, nil, fmt.Errorf("region %q has a %s rating but the dataset has no %s source", region.Key, hazardType, hazardType)
		}
		ratings = append(ratings, domain.HazardRating{
			RegionKey:  region.Key,
			HazardType: hazardType,
			Rating:     rating,
			SourceID:   source.ID,
		})
	}
	if len(ratings) == 0 {
		return domain.HazardRegion{}, nil, fmt.Errorf("region %q has no ratings", region.Key)
	}

	return domain.HazardRegion{
		Key:        region.Key,
		Country:    country,
		Name:       region.Name,
		ParentName: region.ParentName,
		NameSearch: NormalizeName(region.Name),
	}, ratings, nil
}

// NormalizeName lower-cases name, removes diacritics ("Évora" becomes
// "evora") and turns hyphens and runs of whitespace into single spaces, so a
// name typed without accents or hyphens still finds the region. Both the
// stored search column and a search query go through it.
func NormalizeName(name string) string {
	var plain strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(name)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			// combining accent: dropped
		case r == '-':
			plain.WriteByte(' ')
		default:
			plain.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(plain.String()), " ")
}
