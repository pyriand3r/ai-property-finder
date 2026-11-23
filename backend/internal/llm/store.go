package llm

import (
	"context"
	"propertyfinder/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolStore is the Postgres-backed Store: app_config via internal/config.
type PoolStore struct {
	Pool *pgxpool.Pool
}

// Load implements Store.
func (store PoolStore) Load(ctx context.Context) (config.LLM, error) {
	return config.LoadAll(ctx, store.Pool)
}

// Save implements Store.
func (store PoolStore) Save(ctx context.Context, cfg config.LLM, includeAPIKey bool) error {
	return config.SaveLLM(ctx, store.Pool, cfg, includeAPIKey)
}
