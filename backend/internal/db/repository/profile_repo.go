// Package repository holds pgx-backed persistence for the domain
// aggregates. Each file owns one aggregate; every constructor takes a
// db.Querier so a repository can be used standalone (against the pool) or
// inside a transaction (against a pgx.Tx) — see db.WithTx.
package repository

import (
	"context"
	"errors"
	"fmt"
	"propertyfinder/internal/db"
	"propertyfinder/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ProfileRepository persists the search_profile aggregate.
type ProfileRepository interface {
	List(ctx context.Context) ([]domain.SearchProfile, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.SearchProfile, error)
	Create(ctx context.Context, profile *domain.SearchProfile) error
	Update(ctx context.Context, profile *domain.SearchProfile) error
	Archive(ctx context.Context, id uuid.UUID) (*domain.SearchProfile, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// ExistsByName reports whether a profile with a case-insensitively
	// matching name already exists. When excludeID is non-nil, that profile
	// is excluded (used during Update so a profile keeps its own name).
	ExistsByName(ctx context.Context, name string, excludeID *uuid.UUID) (bool, error)
	// SetRankingSummary writes the profile-wide comparison summary (spec
	// 01-05 Layer 5.2 "Rangliste aktualisieren").
	SetRankingSummary(ctx context.Context, profileID uuid.UUID, summary string) error
}

// ProfileRepo is the pgx-backed implementation of ProfileRepository.
type ProfileRepo struct {
	db db.Querier
}

// NewProfileRepo constructs a ProfileRepo wired to q — either the pool or a
// transaction.
func NewProfileRepo(q db.Querier) *ProfileRepo {
	return &ProfileRepo{db: q}
}

// List returns all search profiles, most recently created first.
func (repo *ProfileRepo) List(ctx context.Context) ([]domain.SearchProfile, error) {
	const query = `
SELECT id, name, description, archived_at, ranking_summary, created_at, updated_at
FROM search_profile
ORDER BY created_at DESC
`
	rows, err := repo.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("profile list: %w", err)
	}
	defer rows.Close()

	profiles := make([]domain.SearchProfile, 0)
	for rows.Next() {
		profile, scanError := scanProfileRow(rows)
		if scanError != nil {
			return nil, fmt.Errorf("profile list scan: %w", scanError)
		}
		profiles = append(profiles, profile)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("profile list rows: %w", rowsError)
	}
	return profiles, nil
}

// GetByID returns a single search profile by its UUID.
func (repo *ProfileRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.SearchProfile, error) {
	const query = `
SELECT id, name, description, archived_at, ranking_summary, created_at, updated_at
FROM search_profile
WHERE id = $1
`
	row := repo.db.QueryRow(ctx, query, id)
	profile, err := scanProfileRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "profile", ID: id.String()}
		}
		return nil, fmt.Errorf("profile get: %w", err)
	}
	return &profile, nil
}

// Create inserts a new search profile. The caller generates the UUID and
// timestamps.
func (repo *ProfileRepo) Create(ctx context.Context, profile *domain.SearchProfile) error {
	const query = `
INSERT INTO search_profile (id, name, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
`
	_, err := repo.db.Exec(ctx, query,
		profile.ID,
		profile.Name,
		profile.Description,
		profile.CreatedAt,
		profile.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("profile create: %w", err)
	}
	return nil
}

// Update overwrites name and description and bumps updated_at.
func (repo *ProfileRepo) Update(ctx context.Context, profile *domain.SearchProfile) error {
	const query = `
UPDATE search_profile
SET name = $2, description = $3, updated_at = $4
WHERE id = $1
`
	tag, err := repo.db.Exec(ctx, query,
		profile.ID,
		profile.Name,
		profile.Description,
		profile.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("profile update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "profile", ID: profile.ID.String()}
	}
	return nil
}

// Archive sets archived_at to now and returns the updated profile.
func (repo *ProfileRepo) Archive(ctx context.Context, id uuid.UUID) (*domain.SearchProfile, error) {
	const query = `
UPDATE search_profile
SET archived_at = NOW(), updated_at = NOW()
WHERE id = $1
RETURNING id, name, description, archived_at, ranking_summary, created_at, updated_at
`
	row := repo.db.QueryRow(ctx, query, id)
	profile, err := scanProfileRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "profile", ID: id.String()}
		}
		return nil, fmt.Errorf("profile archive: %w", err)
	}
	return &profile, nil
}

// Delete removes a search profile by its UUID. Cascading FKs handle
// criteria, context, chat history, and listings.
//
// TODO(Layer 3+): once listing images exist, deleting a profile cascades
// listing_image rows at the DB level without removing their files under
// /data/images/listings/{id}/ (ADR 0012 only wires that cleanup into the
// single-listing DELETE endpoint). Revisit when image storage lands.
func (repo *ProfileRepo) Delete(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM search_profile WHERE id = $1`
	tag, err := repo.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("profile delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "profile", ID: id.String()}
	}
	return nil
}

// ExistsByName reports whether a profile with a case-insensitively matching
// name already exists, optionally excluding one profile ID from the check.
func (repo *ProfileRepo) ExistsByName(ctx context.Context, name string, excludeID *uuid.UUID) (bool, error) {
	const query = `
SELECT EXISTS(
  SELECT 1 FROM search_profile
  WHERE LOWER(name) = LOWER($1) AND ($2::uuid IS NULL OR id <> $2)
)
`
	var exists bool
	if err := repo.db.QueryRow(ctx, query, name, excludeID).Scan(&exists); err != nil {
		return false, fmt.Errorf("profile exists by name: %w", err)
	}
	return exists, nil
}

func (repo *ProfileRepo) SetRankingSummary(ctx context.Context, profileID uuid.UUID, summary string) error {
	const query = `UPDATE search_profile SET ranking_summary = $2, updated_at = NOW() WHERE id = $1`
	tag, err := repo.db.Exec(ctx, query, profileID, summary)
	if err != nil {
		return fmt.Errorf("profile set ranking summary: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "profile", ID: profileID.String()}
	}
	return nil
}

// scanProfileRow scans a search_profile row from either a pgx.Row or pgx.Rows.
func scanProfileRow(scanner interface {
	Scan(dest ...any) error
},
) (domain.SearchProfile, error) {
	var profile domain.SearchProfile
	err := scanner.Scan(
		&profile.ID,
		&profile.Name,
		&profile.Description,
		&profile.ArchivedAt,
		&profile.RankingSummary,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	return profile, err
}
