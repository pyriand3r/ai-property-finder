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
