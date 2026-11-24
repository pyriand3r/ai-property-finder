package service

import (
	"context"
	"errors"
	"propertyfinder/internal/db/dbtest"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/hazard"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests cover the hazard dataset sync and lookups (docs/adr/0025,
// issue #44) against a real Postgres (dbtest skips them without
// TEST_DATABASE_URL): the SQL of the sync and the prune statements is only
// exercised here.

func fixtureSource(id string, hazardType domain.HazardType) domain.HazardSource {
	return domain.HazardSource{
		ID: id, HazardType: hazardType, Title: "title " + id, Publisher: "publisher",
		URL: "https://example.test/" + id, License: "license", DatasetVersion: "v1",
		RetrievedOn: "2026-11-24", Method: "method",
	}
}

func fixtureRegion(country, key, name string) domain.HazardRegion {
	return domain.HazardRegion{Key: key, Country: country, Name: name, ParentName: "parent", NameSearch: hazard.NormalizeName(name)}
}

func fixtureRating(key string, hazardType domain.HazardType, rating int, sourceID string) domain.HazardRating {
	return domain.HazardRating{RegionKey: key, HazardType: hazardType, Rating: rating, SourceID: sourceID}
}

func ratingUpdatedAt(t *testing.T, pool *pgxpool.Pool, key string, hazardType domain.HazardType) time.Time {
	t.Helper()
	var updatedAt time.Time
	err := pool.QueryRow(context.Background(),
		`SELECT updated_at FROM hazard_rating WHERE region_key = $1 AND hazard_type = $2`, key, hazardType).Scan(&updatedAt)
	if err != nil {
		t.Fatalf("read updated_at of %s/%s: %v", key, hazardType, err)
	}
	return updatedAt
}

func storedRatings(t *testing.T, pool *pgxpool.Pool) []domain.HazardRating {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT region_key, hazard_type, rating, source_id FROM hazard_rating ORDER BY region_key, hazard_type`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ratings []domain.HazardRating
	for rows.Next() {
		var rating domain.HazardRating
		if scanError := rows.Scan(&rating.RegionKey, &rating.HazardType, &rating.Rating, &rating.SourceID); scanError != nil {
			t.Fatal(scanError)
		}
		ratings = append(ratings, rating)
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	return ratings
}

func storedStrings(t *testing.T, pool *pgxpool.Pool, query string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if scanError := rows.Scan(&value); scanError != nil {
			t.Fatal(scanError)
		}
		values = append(values, value)
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	return values
}

// TestHazardService_SyncDataset_MirrorsTheDatasetAndLeavesUnchangedRowsAlone
// pins the two promises of "tables are a cache of the files": a boot with
// unchanged data does not touch updated_at, and a changed dataset leaves
// nothing of the old one behind — except other countries' data.
func TestHazardService_SyncDataset_MirrorsTheDatasetAndLeavesUnchangedRowsAlone(t *testing.T) {
	pool := dbtest.NewPool(t)
	service := NewHazardService(pool)
	ctx := context.Background()
	wildfire, earthquake := domain.HazardWildfire, domain.HazardEarthquake

	austria := hazard.Dataset{
		Country: "AT",
		Sources: []domain.HazardSource{fixtureSource("at-fire", wildfire)},
		Regions: []domain.HazardRegion{fixtureRegion("AT", "AT-10101", "Eisenstadt")},
		Ratings: []domain.HazardRating{fixtureRating("AT-10101", wildfire, 2, "at-fire")},
	}
	releaseOne := hazard.Dataset{
		Country: "PT",
		Sources: []domain.HazardSource{fixtureSource("fire-v1", wildfire), fixtureSource("quake-v1", earthquake)},
		Regions: []domain.HazardRegion{fixtureRegion("PT", "PT-0001", "Alfa"), fixtureRegion("PT", "PT-0002", "Beta")},
		Ratings: []domain.HazardRating{
			fixtureRating("PT-0001", wildfire, 4, "fire-v1"),
			fixtureRating("PT-0001", earthquake, 3, "quake-v1"),
			fixtureRating("PT-0002", wildfire, 1, "fire-v1"),
		},
	}
	// Release two: Alfa's wildfire rating changes and its earthquake rating
	// is dropped, Beta is gone, Gamma is new, the wildfire source has a new
	// id and the earthquake source is no longer used.
	releaseTwo := hazard.Dataset{
		Country: "PT",
		Sources: []domain.HazardSource{fixtureSource("fire-v2", wildfire)},
		Regions: []domain.HazardRegion{fixtureRegion("PT", "PT-0001", "Alfa"), fixtureRegion("PT", "PT-0003", "Gamma")},
		Ratings: []domain.HazardRating{
			fixtureRating("PT-0001", wildfire, 5, "fire-v2"),
			fixtureRating("PT-0003", wildfire, 2, "fire-v2"),
		},
	}

	for _, dataset := range []hazard.Dataset{austria, releaseOne} {
		if err := service.syncDataset(ctx, dataset); err != nil {
			t.Fatalf("sync %s: %v", dataset.Country, err)
		}
	}
	stamp := ratingUpdatedAt(t, pool, "PT-0001", wildfire)
	if err := service.syncDataset(ctx, releaseOne); err != nil {
		t.Fatalf("second sync of identical data: %v", err)
	}
	if got := ratingUpdatedAt(t, pool, "PT-0001", wildfire); !got.Equal(stamp) {
		t.Errorf("updated_at moved from %v to %v although nothing changed", stamp, got)
	}

	if err := service.syncDataset(ctx, releaseTwo); err != nil {
		t.Fatalf("sync release two: %v", err)
	}

	wantRatings := []domain.HazardRating{
		fixtureRating("AT-10101", wildfire, 2, "at-fire"),
		fixtureRating("PT-0001", wildfire, 5, "fire-v2"),
		fixtureRating("PT-0003", wildfire, 2, "fire-v2"),
	}
	if got := storedRatings(t, pool); !slices.Equal(got, wantRatings) {
		t.Errorf("ratings = %+v, want %+v", got, wantRatings)
	}
	if got, want := storedStrings(t, pool, `SELECT key FROM hazard_region ORDER BY key`), []string{"AT-10101", "PT-0001", "PT-0003"}; !slices.Equal(got, want) {
		t.Errorf("regions = %v, want %v (Beta pruned, Austria untouched)", got, want)
	}
	if got, want := storedStrings(t, pool, `SELECT id FROM hazard_source ORDER BY id`), []string{"at-fire", "fire-v2"}; !slices.Equal(got, want) {
		t.Errorf("sources = %v, want %v (the unused ones pruned, Austria's kept)", got, want)
	}
	if got := ratingUpdatedAt(t, pool, "PT-0001", wildfire); !got.After(stamp) {
		t.Errorf("updated_at = %v, want it after %v: the rating changed from 4 to 5", got, stamp)
	}
}

func TestHazardService_SeedStatic_LoadsEmbeddedDatasetsAndIsIdempotent(t *testing.T) {
	pool := dbtest.NewPool(t)
	service := NewHazardService(pool)
	ctx := context.Background()

	if err := service.SeedStatic(ctx); err != nil {
		t.Fatalf("SeedStatic: %v", err)
	}

	datasets, err := hazard.Load()
	if err != nil {
		t.Fatal(err)
	}
	wantRatings := 0
	for _, dataset := range datasets {
		wantRatings += len(dataset.Ratings)
	}
	before := storedRatings(t, pool)
	if len(before) != wantRatings {
		t.Fatalf("%d ratings stored, want the %d the embedded datasets hold", len(before), wantRatings)
	}

	stamp := ratingUpdatedAt(t, pool, before[0].RegionKey, before[0].HazardType)
	if err = service.SeedStatic(ctx); err != nil {
		t.Fatalf("second SeedStatic (a reboot): %v", err)
	}
	if got := ratingUpdatedAt(t, pool, before[0].RegionKey, before[0].HazardType); !got.Equal(stamp) {
		t.Errorf("a reboot moved updated_at from %v to %v", stamp, got)
	}
}

func TestHazardService_GetRegionHazards_ReturnsRatingsInHazardOrderWithSources(t *testing.T) {
	pool := dbtest.NewPool(t)
	service := NewHazardService(pool)
	ctx := context.Background()
	// Inserted in alphabetical order on purpose: the API order is the
	// domain's (wildfire, flood, earthquake, landslide), not the database's.
	dataset := hazard.Dataset{
		Country: "PT",
		Sources: []domain.HazardSource{
			fixtureSource("s-quake", domain.HazardEarthquake),
			fixtureSource("s-fire", domain.HazardWildfire),
		},
		Regions: []domain.HazardRegion{fixtureRegion("PT", "PT-0001", "Alfa")},
		Ratings: []domain.HazardRating{
			fixtureRating("PT-0001", domain.HazardEarthquake, 3, "s-quake"),
			fixtureRating("PT-0001", domain.HazardWildfire, 4, "s-fire"),
		},
	}
	if err := service.syncDataset(ctx, dataset); err != nil {
		t.Fatal(err)
	}

	got, err := service.GetRegionHazards(ctx, " pt-0001 ")
	if err != nil {
		t.Fatalf("GetRegionHazards: %v", err)
	}

	if got.Key != "PT-0001" || got.Name != "Alfa" || got.Country != "PT" {
		t.Errorf("region = %+v", got.HazardRegion)
	}
	if len(got.Ratings) != 2 ||
		got.Ratings[0].HazardType != domain.HazardWildfire || got.Ratings[0].Rating != 4 || got.Ratings[0].Source.ID != "s-fire" ||
		got.Ratings[1].HazardType != domain.HazardEarthquake || got.Ratings[1].Rating != 3 || got.Ratings[1].Source.Title != "title s-quake" {
		t.Errorf("ratings = %+v, want wildfire then earthquake, each with its own source", got.Ratings)
	}
	if got.Ratings[0].Source.RetrievedOn != "2026-11-24" {
		t.Errorf("source retrieved_on = %q, want the plain date 2026-11-24", got.Ratings[0].Source.RetrievedOn)
	}
}

func TestHazardService_Lookups_RejectBadInputAndUnknownKeys(t *testing.T) {
	pool := dbtest.NewPool(t)
	service := NewHazardService(pool)
	ctx := context.Background()

	var notFound *domain.NotFoundError
	if _, err := service.GetRegionHazards(ctx, "PT-9999"); !errors.As(err, &notFound) {
		t.Errorf("unknown key: err = %v, want *domain.NotFoundError", err)
	}
	var invalid *domain.ValidationError
	if _, err := service.GetRegionHazards(ctx, "lisboa"); !errors.As(err, &invalid) {
		t.Errorf("malformed key: err = %v, want *domain.ValidationError", err)
	}
	if _, err := service.SearchRegions(ctx, "Portugal", ""); !errors.As(err, &invalid) {
		t.Errorf("country %q: err = %v, want *domain.ValidationError", "Portugal", err)
	}
}

// TestHazardService_SearchRegions_IgnoresCaseAccentsAndWildcards: the names
// that matter are "Évora", "Lagoa" (two municipalities) and the hyphenated
// ones; "%" typed by a caller must stay a plain character.
func TestHazardService_SearchRegions_IgnoresCaseAccentsAndWildcards(t *testing.T) {
	pool := dbtest.NewPool(t)
	service := NewHazardService(pool)
	ctx := context.Background()
	wildfire := domain.HazardWildfire
	regions := []domain.HazardRegion{
		fixtureRegion("PT", "PT-0705", "Évora"),
		fixtureRegion("PT", "PT-0806", "Lagoa"),
		fixtureRegion("PT", "PT-4201", "Lagoa"),
		fixtureRegion("PT", "PT-0102", "Albergaria-a-Velha"),
	}
	ratings := make([]domain.HazardRating, len(regions))
	for i, region := range regions {
		ratings[i] = fixtureRating(region.Key, wildfire, 1, "s")
	}
	dataset := hazard.Dataset{
		Country: "PT", Sources: []domain.HazardSource{fixtureSource("s", wildfire)}, Regions: regions, Ratings: ratings,
	}
	if err := service.syncDataset(ctx, dataset); err != nil {
		t.Fatal(err)
	}

	keys := func(country, query string) []string {
		t.Helper()
		found, err := service.SearchRegions(ctx, country, query)
		if err != nil {
			t.Fatalf("SearchRegions(%q, %q): %v", country, query, err)
		}
		result := make([]string, len(found))
		for i, region := range found {
			result[i] = region.Key
		}
		return result
	}
	cases := []struct {
		country, query string
		want           []string
	}{
		{"PT", "evora", []string{"PT-0705"}},
		{"pt", "ÉVORA", []string{"PT-0705"}},
		{"", "lagoa", []string{"PT-0806", "PT-4201"}},
		{"PT", "albergaria a velha", []string{"PT-0102"}},
		{"PT", "%", nil},
		{"AT", "evora", nil},
		{"PT", "", []string{"PT-0102", "PT-0705", "PT-0806", "PT-4201"}},
	}
	for _, tc := range cases {
		got := keys(tc.country, tc.query)
		if !slices.Equal(got, tc.want) && (len(got) != 0 || len(tc.want) != 0) {
			t.Errorf("SearchRegions(%q, %q) = %v, want %v", tc.country, tc.query, got, tc.want)
		}
	}
}
