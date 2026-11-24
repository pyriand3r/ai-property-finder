package db_test

import (
	"context"
	"database/sql"
	"propertyfinder/internal/db"
	"propertyfinder/internal/db/dbtest"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	mpgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	migrationsfs "propertyfinder/internal/db/migrations"
)

// migrateUpTo applies migrations 1..version only, like a database that last
// booted an older release.
func migrateUpTo(t *testing.T, databaseURL string, version uint) {
	t.Helper()
	source, err := iofs.New(migrationsfs.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	stdDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer stdDB.Close()
	driver, err := mpgx.WithInstance(stdDB, &mpgx.Config{})
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(version); err != nil {
		t.Fatalf("migrate to %d: %v", version, err)
	}
}

// TestMigration000004_ExistingCriteriaBecomeActive is the risk named in
// issue #29: if the new status column did not default to 'active', every
// profile would suddenly evaluate against zero criteria after the upgrade.
func TestMigration000004_ExistingCriteriaBecomeActive(t *testing.T) {
	const profileID = "00000000-0000-0000-0000-000000000001"
	databaseURL := dbtest.NewURL(t)
	migrateUpTo(t, databaseURL, 3)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `INSERT INTO search_profile (id, name) VALUES ($1, 'P')`, profileID)
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO criterion (profile_id, name, description, is_subjective) VALUES ($1, 'Preis', '', false)`, profileID)
	if err != nil {
		t.Fatalf("seed criterion on the version-3 schema: %v", err)
	}

	if err = db.Run(ctx, databaseURL); err != nil {
		t.Fatalf("db.Run on a version-3 database: %v", err)
	}

	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM criterion WHERE name = 'Preis'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		t.Errorf("pre-existing criterion status = %q, want active", status)
	}

	const insert = `INSERT INTO criterion (profile_id, name, description, is_subjective, status) VALUES ($1, $2, '', false, $3)`
	if _, err = pool.Exec(ctx, insert, profileID, "Lage", "proposed"); err != nil {
		t.Errorf("insert proposed criterion: %v", err)
	}
	if _, err = pool.Exec(ctx, insert, profileID, "X", "bogus"); err == nil {
		t.Error("status 'bogus' was accepted, want the CHECK constraint to reject it")
	}
}

// TestMigration000005_HazardTablesEnforceTheirConstraints: the repository
// tests never run SQL, so nothing else would notice a mistyped constraint.
// Each rejected row below is a data error a hand-edited dataset could carry;
// the last block checks the one delete that must cascade.
func TestMigration000005_HazardTablesEnforceTheirConstraints(t *testing.T) {
	pool := dbtest.NewPool(t)
	ctx := context.Background()

	const (
		insertSource = `INSERT INTO hazard_source (id, hazard_type, title, publisher, url, license, dataset_version, retrieved_on, method)
			VALUES ($1, $2, 't', 'p', 'https://example.test', 'l', 'v', '2026-11-24', 'm')`
		insertRegion = `INSERT INTO hazard_region (key, country, name, name_search) VALUES ($1, $2, 'n', 'n')`
		insertRating = `INSERT INTO hazard_rating (region_key, hazard_type, rating, source_id) VALUES ($1, $2, $3, $4)`
	)
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s %v: %v", sql, args, err)
		}
	}
	mustExec(insertSource, "fire", "wildfire")
	mustExec(insertSource, "quake", "earthquake")
	mustExec(insertRegion, "PT-1106", "PT")
	mustExec(insertRating, "PT-1106", "wildfire", 5, "fire")

	rejected := map[string]struct {
		sql  string
		args []any
	}{
		"rating 6 is above the scale":           {insertRating, []any{"PT-1106", "earthquake", 6, "quake"}},
		"rating 0 is below the scale":           {insertRating, []any{"PT-1106", "earthquake", 0, "quake"}},
		"rating cites a source of another type": {insertRating, []any{"PT-1106", "earthquake", 3, "fire"}},
		"rating cites an unknown source":        {insertRating, []any{"PT-1106", "earthquake", 3, "nope"}},
		"rating for an unknown region":          {insertRating, []any{"PT-9999", "wildfire", 3, "fire"}},
		"second rating for the same hazard":     {insertRating, []any{"PT-1106", "wildfire", 2, "fire"}},
		"source with an unknown hazard type":    {insertSource, []any{"tsunami-src", "tsunami"}},
		"region key in lower case":              {insertRegion, []any{"pt-1", "PT"}},
		"region key of another country":         {insertRegion, []any{"ES-1", "PT"}},
		"region country that is no two-letter":  {insertRegion, []any{"PRT-1", "PRT"}},
	}
	for name, tc := range rejected {
		if _, err := pool.Exec(ctx, tc.sql, tc.args...); err == nil {
			t.Errorf("%s: accepted, want the constraint to reject it", name)
		}
	}

	mustExec(`DELETE FROM hazard_region WHERE key = 'PT-1106'`)
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM hazard_rating`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d ratings left after deleting their region, want 0 (ON DELETE CASCADE)", left)
	}
}
