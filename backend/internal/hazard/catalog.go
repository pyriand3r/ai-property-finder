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
	"strings"
)

// catalogFiles holds one JSON file per country: the declarative list of
// hazard data sources known for it, shipped or not (docs/adr/0027, spec
// 01-15). Adding a source is a change to one of these files.
//
//go:embed catalog/*.json
var catalogFiles embed.FS

// catalogFile is the on-disk format of a country's catalog.
type catalogFile struct {
	Country string                      `json:"country"`
	Entries []domain.HazardCatalogEntry `json:"entries"`
}

// LoadCatalog parses and validates every embedded catalog file against the
// embedded datasets, and returns all entries in file order (files by name).
// A catalog that fails validation stops the backend at boot, like a bad
// dataset.
func LoadCatalog() ([]domain.HazardCatalogEntry, error) {
	datasets, err := Load()
	if err != nil {
		return nil, err
	}
	names, err := fs.Glob(catalogFiles, "catalog/*.json")
	if err != nil {
		return nil, fmt.Errorf("list hazard catalog files: %w", err)
	}
	var all []domain.HazardCatalogEntry
	countries := make(map[string]string, len(names))
	ids := make(map[string]string)
	for _, name := range names {
		data, readError := catalogFiles.ReadFile(name)
		if readError != nil {
			return nil, fmt.Errorf("read %s: %w", name, readError)
		}
		entries, parseError := ParseCatalog(data, datasets)
		if parseError != nil {
			return nil, fmt.Errorf("%s: %w", name, parseError)
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("%s: no entries", name)
		}
		country := entries[0].Country
		if other, seen := countries[country]; seen {
			return nil, fmt.Errorf("%s and %s both hold the catalog for %s", other, name, country)
		}
		countries[country] = name
		for _, entry := range entries {
			if other, seen := ids[entry.ID]; seen {
				return nil, fmt.Errorf("%s: entry id %q is already used in %s", name, entry.ID, other)
			}
			ids[entry.ID] = name
		}
		all = append(all, entries...)
	}
	for _, dataset := range datasets {
		if _, ok := countries[dataset.Country]; !ok {
			return nil, fmt.Errorf("dataset %s has no catalog file", dataset.Country)
		}
	}
	return all, nil
}

// ParseCatalog decodes one catalog file and checks it against datasets:
//
//   - unknown fields, data after the object, a bad country, an empty list;
//   - an entry id that is empty, repeated or not prefixed with the lower-case
//     country code ("pt-"), an unknown hazard type or status, a missing scope
//     (use "Portugal" style text for the whole country);
//   - the same (hazard type, URL) twice;
//   - a bundled entry must name a source of the country's dataset with the
//     same hazard type, and must not restate title, publisher, URL or license
//     (they are copied from the source); every dataset source must be named by
//     exactly one bundled entry, so no shipped data is missing from the
//     catalog;
//   - a candidate entry needs title, publisher, an http(s) URL and notes (what
//     is still unknown), and must not name a source.
//
// Entries are returned with Country set and bundled fields filled in.
func ParseCatalog(data []byte, datasets []Dataset) ([]domain.HazardCatalogEntry, error) {
	var file catalogFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("decode: unexpected data after the catalog object")
	}
	if !ValidCountry(file.Country) {
		return nil, fmt.Errorf("country %q is not an ISO 3166-1 alpha-2 code", file.Country)
	}

	sources := map[string]domain.HazardSource{}
	for _, dataset := range datasets {
		if dataset.Country == file.Country {
			for _, source := range dataset.Sources {
				sources[source.ID] = source
			}
		}
	}

	prefix := strings.ToLower(file.Country) + "-"
	ids := make(map[string]bool, len(file.Entries))
	urlKeys := make(map[string]string, len(file.Entries))
	bundled := make(map[string]string, len(sources))
	entries := make([]domain.HazardCatalogEntry, 0, len(file.Entries))
	for _, entry := range file.Entries {
		entry.Country = file.Country
		if entryError := validateCatalogEntry(entry, prefix, ids); entryError != nil {
			return nil, fmt.Errorf("entry %q: %w", entry.ID, entryError)
		}
		ids[entry.ID] = true

		switch entry.Status {
		case domain.HazardCatalogBundled:
			resolved, resolveError := resolveBundled(entry, sources, bundled)
			if resolveError != nil {
				return nil, fmt.Errorf("entry %q: %w", entry.ID, resolveError)
			}
			entry = resolved
		case domain.HazardCatalogCandidate:
			if candidateError := validateCandidate(entry); candidateError != nil {
				return nil, fmt.Errorf("entry %q: %w", entry.ID, candidateError)
			}
		}

		key := string(entry.HazardType) + " " + SourceURLKey(entry.URL)
		if other, seen := urlKeys[key]; seen {
			return nil, fmt.Errorf("entry %q has the same hazard type and URL as %q", entry.ID, other)
		}
		urlKeys[key] = entry.ID
		entries = append(entries, entry)
	}
	for id := range sources {
		if _, ok := bundled[id]; !ok {
			return nil, fmt.Errorf("dataset source %q has no bundled catalog entry", id)
		}
	}
	return entries, nil
}

func validateCatalogEntry(entry domain.HazardCatalogEntry, idPrefix string, ids map[string]bool) error {
	if !strings.HasPrefix(entry.ID, idPrefix) || len(entry.ID) == len(idPrefix) {
		return fmt.Errorf("id must start with %q", idPrefix)
	}
	if ids[entry.ID] {
		return errors.New("id appears twice")
	}
	if !entry.HazardType.Valid() {
		return fmt.Errorf("unknown hazard type %q", entry.HazardType)
	}
	if strings.TrimSpace(entry.Scope) == "" {
		return errors.New("scope is empty")
	}
	if entry.Status != domain.HazardCatalogBundled && entry.Status != domain.HazardCatalogCandidate {
		return fmt.Errorf("unknown status %q", entry.Status)
	}
	return nil
}

func resolveBundled(
	entry domain.HazardCatalogEntry,
	sources map[string]domain.HazardSource,
	bundled map[string]string,
) (domain.HazardCatalogEntry, error) {
	if entry.Title != "" || entry.Publisher != "" || entry.URL != "" || entry.License != "" {
		return entry, errors.New("a bundled entry must not state title, publisher, url or license: they come from the dataset source")
	}
	source, ok := sources[entry.SourceID]
	if !ok {
		return entry, fmt.Errorf("source_id %q is not a source of the %s dataset", entry.SourceID, entry.Country)
	}
	if source.HazardType != entry.HazardType {
		return entry, fmt.Errorf("source %q is a %s source, the entry says %s", source.ID, source.HazardType, entry.HazardType)
	}
	if other, seen := bundled[source.ID]; seen {
		return entry, fmt.Errorf("source %q is already named by entry %q", source.ID, other)
	}
	bundled[source.ID] = entry.ID
	entry.Title, entry.Publisher, entry.URL, entry.License = source.Title, source.Publisher, source.URL, source.License
	return entry, nil
}

func validateCandidate(entry domain.HazardCatalogEntry) error {
	if entry.SourceID != "" {
		return errors.New("a candidate has no dataset source (source_id)")
	}
	for _, field := range []struct{ name, value string }{
		{"title", entry.Title}, {"publisher", entry.Publisher}, {"notes", entry.Notes},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is empty", field.name)
		}
	}
	if SourceURLKey(entry.URL) == "" {
		return fmt.Errorf("url %q is not an http(s) URL", entry.URL)
	}
	return nil
}

// SourceURLKey is the identity of a source URL: lower-cased host without
// "www.", no scheme (http and https are the same source), no fragment, no
// trailing slash, query kept. "" means raw is not an http(s) URL. A new
// suggestion is compared with the catalog and with earlier suggestions by
// this key.
func SourceURLKey(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	if port := parsed.Port(); port != "" {
		host += ":" + port
	}
	key := host + strings.TrimRight(parsed.EscapedPath(), "/")
	if parsed.RawQuery != "" {
		key += "?" + parsed.RawQuery
	}
	return key
}
