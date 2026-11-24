package repository

import (
	"encoding/json"
	"errors"
	"propertyfinder/internal/domain"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestHazardRepo_ListRatingsByRegion_ScansRatingThenSourceColumns(t *testing.T) {
	q := &fakeQuerier{rowsValues: [][]any{{
		domain.HazardWildfire, 4, someTime,
		"pt-fire", domain.HazardWildfire, "Fire map", "ICNF", "https://example.test/fire", "CC BY 4.0", "2020", "2026-11-24", "mean class", "note",
	}}}

	ratings, err := NewHazardRepo(q).ListRatingsByRegion(ctx, "PT-1106")
	if err != nil {
		t.Fatalf("ListRatingsByRegion: %v", err)
	}

	want := domain.HazardRatingDetail{
		HazardType: domain.HazardWildfire, Rating: 4, UpdatedAt: someTime,
		Source: domain.HazardSource{
			ID: "pt-fire", HazardType: domain.HazardWildfire, Title: "Fire map", Publisher: "ICNF",
			URL: "https://example.test/fire", License: "CC BY 4.0", DatasetVersion: "2020",
			RetrievedOn: "2026-11-24", Method: "mean class", Notes: "note",
		},
	}
	if len(ratings) != 1 || ratings[0] != want {
		t.Errorf("ratings = %+v, want [%+v]", ratings, want)
	}
	if len(q.args) != 1 || q.args[0] != "PT-1106" {
		t.Errorf("query args = %v, want [PT-1106]", q.args)
	}
}

func TestHazardRepo_SearchRegions_BindsCountryThenNameSearch(t *testing.T) {
	q := &fakeQuerier{rowsValues: [][]any{
		{"PT-0806", "PT", "Lagoa", "Faro"},
		{"PT-4201", "PT", "Lagoa", "Ilha de São Miguel"},
	}}

	regions, err := NewHazardRepo(q).SearchRegions(ctx, "PT", "lagoa")
	if err != nil {
		t.Fatalf("SearchRegions: %v", err)
	}

	if len(q.args) != 2 || q.args[0] != "PT" || q.args[1] != "lagoa" {
		t.Errorf("query args = %v, want [PT lagoa]", q.args)
	}
	if len(regions) != 2 || regions[1].Key != "PT-4201" || regions[1].ParentName != "Ilha de São Miguel" {
		t.Errorf("regions = %+v, want both Lagoa regions with their parents, in row order", regions)
	}
}

func TestHazardRepo_UpsertRegions_SendsOneJSONArrayIncludingNameSearch(t *testing.T) {
	q := &fakeQuerier{}
	regions := []domain.HazardRegion{
		{Key: "PT-0705", Country: "PT", Name: "Évora", ParentName: "Évora", NameSearch: "evora"},
		{Key: "PT-0502", Country: "PT", Name: "Castelo Branco", ParentName: "Castelo Branco", NameSearch: "castelo branco"},
	}

	if err := NewHazardRepo(q).UpsertRegions(ctx, regions); err != nil {
		t.Fatalf("UpsertRegions: %v", err)
	}

	if len(q.execSQL) != 1 || len(q.execArgs[0]) != 1 {
		t.Fatalf("got %d statements with args %v, want one statement with one argument", len(q.execSQL), q.execArgs)
	}
	payload, ok := q.execArgs[0][0].(json.RawMessage)
	if !ok {
		t.Fatalf("argument is %T, want json.RawMessage (bound to a jsonb parameter)", q.execArgs[0][0])
	}
	var rows []map[string]string
	if err := json.Unmarshal(payload, &rows); err != nil {
		t.Fatalf("payload %s is not a JSON array of objects: %v", payload, err)
	}
	if len(rows) != 2 || rows[0]["key"] != "PT-0705" || rows[0]["name_search"] != "evora" || rows[1]["parent_name"] != "Castelo Branco" {
		t.Errorf("rows = %v, want both regions in order, with the name_search column the sync statement reads", rows)
	}
}

func TestHazardRepo_Upserts_WithNoRowsDoNotTouchTheDatabase(t *testing.T) {
	q := &fakeQuerier{}
	repo := NewHazardRepo(q)

	for name, err := range map[string]error{
		"sources": repo.UpsertSources(ctx, nil),
		"regions": repo.UpsertRegions(ctx, nil),
		"ratings": repo.UpsertRatings(ctx, []domain.HazardRating{}),
	} {
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(q.execSQL) != 0 {
		t.Errorf("ran %d statements for empty input, want none", len(q.execSQL))
	}
}

// TestHazardRepo_Prune_EmptyKeepListMeansKeepNothing: a nil slice reaches
// Postgres as NULL, and "key <> ALL(NULL)" is NULL — the prune would silently
// delete nothing. Both prunes must send an explicit empty array instead.
func TestHazardRepo_Prune_EmptyKeepListMeansKeepNothing(t *testing.T) {
	q := &fakeQuerier{}
	repo := NewHazardRepo(q)

	if err := repo.PruneRegions(ctx, "PT", nil); err != nil {
		t.Fatalf("PruneRegions: %v", err)
	}
	if keep, ok := q.execArgs[0][1].([]string); !ok || keep == nil || len(keep) != 0 {
		t.Errorf("PruneRegions keep arg = %#v, want a non-nil empty []string", q.execArgs[0][1])
	}

	if err := repo.PruneRatings(ctx, "PT", nil); err != nil {
		t.Fatalf("PruneRatings: %v", err)
	}
	if got, ok := q.execArgs[1][1].(json.RawMessage); !ok || string(got) != "[]" {
		t.Errorf("PruneRatings keep arg = %#v, want the JSON array []", q.execArgs[1][1])
	}
}

func TestHazardRepo_SyncWrites_WrapDatabaseErrors(t *testing.T) {
	one := []domain.HazardRating{{RegionKey: "PT-0502", HazardType: domain.HazardWildfire, Rating: 3, SourceID: "s"}}
	writes := map[string]func(*HazardRepo) error{
		"upsert sources": func(r *HazardRepo) error {
			return r.UpsertSources(ctx, []domain.HazardSource{{ID: "s"}})
		},
		"upsert regions": func(r *HazardRepo) error {
			return r.UpsertRegions(ctx, []domain.HazardRegion{{Key: "PT-0502"}})
		},
		"upsert ratings": func(r *HazardRepo) error { return r.UpsertRatings(ctx, one) },
		"prune ratings":  func(r *HazardRepo) error { return r.PruneRatings(ctx, "PT", one) },
		"prune regions":  func(r *HazardRepo) error { return r.PruneRegions(ctx, "PT", []string{"PT-0502"}) },
		"prune sources":  func(r *HazardRepo) error { return r.PruneSources(ctx) },
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			if err := write(NewHazardRepo(&fakeQuerier{})); err != nil {
				t.Errorf("success path: %v", err)
			}
			if err := write(NewHazardRepo(&fakeQuerier{execErr: errBoom})); !errors.Is(err, errBoom) {
				t.Errorf("exec error → %v, want wrapped %v", err, errBoom)
			}
		})
	}
}

// The by-key lookup and the two read queries follow the repository-wide
// error contract that repository_test.go pins for the other repositories:
// no row is a 404, any other database error stays a wrapped 500.
func TestHazardRepo_GetRegion_MapsErrNoRowsToNotFound(t *testing.T) {
	_, err := NewHazardRepo(&fakeQuerier{rowErr: pgx.ErrNoRows}).GetRegion(ctx, "PT-9999")
	if !isNotFound(err) {
		t.Errorf("ErrNoRows → %v, want *domain.NotFoundError", err)
	}
	_, err = NewHazardRepo(&fakeQuerier{rowErr: errBoom}).GetRegion(ctx, "PT-1106")
	if isNotFound(err) || !errors.Is(err, errBoom) {
		t.Errorf("other error → %v, want a wrapped %v that is not NotFound", err, errBoom)
	}
}

func TestHazardRepo_ReadQueries_PropagateQueryScanAndIterationErrors(t *testing.T) {
	reads := map[string]func(*fakeQuerier) error{
		"search regions": func(q *fakeQuerier) error { _, err := NewHazardRepo(q).SearchRegions(ctx, "PT", "x"); return err },
		"list ratings":   func(q *fakeQuerier) error { _, err := NewHazardRepo(q).ListRatingsByRegion(ctx, "PT-1106"); return err },
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			if err := read(&fakeQuerier{queryErr: errBoom}); !errors.Is(err, errBoom) {
				t.Errorf("query error → %v, want wrapped %v", err, errBoom)
			}
			if err := read(&fakeQuerier{rowsValues: [][]any{{"wrong", "arity"}}}); err == nil {
				t.Error("scan failure swallowed")
			}
			if err := read(&fakeQuerier{rowsErr: errBoom}); !errors.Is(err, errBoom) {
				t.Errorf("rows.Err → %v, want wrapped %v", err, errBoom)
			}
		})
	}
}
