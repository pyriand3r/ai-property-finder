// Package dbtest gives integration tests a throwaway, fully migrated
// Postgres database. Tests that use it are skipped unless TEST_DATABASE_URL
// points at a Postgres server the test may create databases on (CI sets it;
// locally: any scratch server). Each call creates its own database, so tests
// never see each other's rows.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"propertyfinder/internal/db"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewURL creates an empty database and returns its connection URL. It drops
// the database when the test ends. Skips the test if TEST_DATABASE_URL is unset.
func NewURL(t *testing.T) string {
	t.Helper()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("random database name: %v", err)
	}
	name := "pf_test_" + hex.EncodeToString(suffix)

	ctx := context.Background()
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL: %v", err)
	}
	if _, execErr := admin.Exec(ctx, "CREATE DATABASE "+name); execErr != nil {
		admin.Close()
		t.Fatalf("create test database: %v", execErr)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
		admin.Close()
	})

	parsed, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	parsed.Path = "/" + name
	return parsed.String()
}

// NewPool creates a database via NewURL, applies every migration, and
// returns a pool on it.
func NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := NewURL(t)
	if err := db.Run(context.Background(), databaseURL); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
