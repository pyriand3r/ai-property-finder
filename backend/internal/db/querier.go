package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is satisfied by both *pgxpool.Pool and pgx.Tx, so repositories can
// be constructed against either — the normal pool for standalone calls, or a
// transaction when an operation must span multiple repositories atomically
// (e.g. creating a profile, its chat session, and its Profile Context
// together).
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// WithTx runs fn inside a transaction, committing on success and rolling
// back on any error (including a panic, which it re-panics after rollback).
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback(ctx)
			panic(recovered)
		}
	}()

	if fnError := fn(tx); fnError != nil {
		_ = tx.Rollback(ctx)
		return fnError
	}
	if commitError := tx.Commit(ctx); commitError != nil {
		return fmt.Errorf("commit transaction: %w", commitError)
	}
	return nil
}
