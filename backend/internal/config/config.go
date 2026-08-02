// Package config resolves environment-only startup settings. Tier 2
// (DB-backed app_config, e.g. llm.*) is introduced in a later layer once
// something actually reads it.
package config

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Env holds settings read from the environment on every startup and never
// stored in the database. Missing required values cause LoadEnv to fail
// fast.
type Env struct {
	// DatabaseURL is the PostgreSQL connection string (always env, never DB).
	DatabaseURL string
	// HTTPListenAddr is the address the HTTP server binds to (default ":8080").
	HTTPListenAddr string
}

// LoadEnv reads Env from environ, a slice of "KEY=VALUE" strings such as
// os.Environ(). DATABASE_URL is required; HTTP_LISTEN_ADDR defaults to
// ":8080" when absent or blank.
func LoadEnv(environ []string) (Env, error) {
	values := parseEnviron(environ)

	databaseURL := strings.TrimSpace(values["DATABASE_URL"])
	if databaseURL == "" {
		return Env{}, fmt.Errorf("DATABASE_URL is required")
	}

	return Env{
		DatabaseURL:    databaseURL,
		HTTPListenAddr: envOrDefault(values, "HTTP_LISTEN_ADDR", ":8080"),
	}, nil
}

// ConnectPool builds a pgxpool connected to databaseURL with sane defaults
// for a single-user, local-network app.
func ConnectPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if pingError := pool.Ping(ctx); pingError != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", pingError)
	}
	return pool, nil
}

func parseEnviron(environ []string) map[string]string {
	values := make(map[string]string, len(environ))
	for _, entry := range environ {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		values[key] = value
	}
	return values
}

func envOrDefault(values map[string]string, key, fallback string) string {
	if value := strings.TrimSpace(values[key]); value != "" {
		return value
	}
	return fallback
}
