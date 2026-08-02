// Package db runs the embedded SQL migrations against the application
// database on startup.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	mpgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	migrationsfs "propertyfinder/internal/db/migrations"
)

// Run applies all pending up migrations embedded in the binary against the
// database identified by databaseURL. Migrations are forward-only; on
// failure the caller must exit non-zero so the Docker restart policy
// surfaces the failure.
//
// The pgx golang-migrate driver consumes a *sql.DB via the pgx stdlib driver
// registered by the blank import above; database/sql connections are pooled
// independently of the application's pgxpool.
func Run(ctx context.Context, databaseURL string) error {
	sourceDriver, err := iofs.New(migrationsfs.Files, ".")
	if err != nil {
		return fmt.Errorf("embed migration source: %w", err)
	}

	stdDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		sourceDriver.Close()
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer stdDB.Close()

	if pingError := stdDB.PingContext(ctx); pingError != nil {
		sourceDriver.Close()
		return fmt.Errorf("ping database for migrations: %w", pingError)
	}

	driver, err := mpgx.WithInstance(stdDB, &mpgx.Config{})
	if err != nil {
		sourceDriver.Close()
		return fmt.Errorf("build migration driver: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", sourceDriver, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("build migrator: %w", err)
	}
	defer migrator.Close()

	if migrationError := migrator.Up(); migrationError != nil && !errors.Is(migrationError, migrate.ErrNoChange) {
		return migrationError
	}
	return nil
}
