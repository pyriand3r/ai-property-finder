package repository

import (
	"context"
	"errors"
	"fmt"
	"propertyfinder/internal/db"
	"propertyfinder/internal/domain"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// HazardSuggestionRepository persists hazard source suggestions (docs/adr/
// 0027). Unlike the static hazard_* tables these rows are written at runtime.
type HazardSuggestionRepository interface {
	// Create inserts a suggestion. The caller sets the UUID, status and
	// timestamp, and urlKey (hazard.SourceURLKey of its URL). A second
	// suggestion for the same (country, hazard type, urlKey) violates the
	// unique constraint.
	Create(ctx context.Context, suggestion *domain.HazardSourceSuggestion, urlKey string) error
	// List returns suggestions, newest first; status "" means all.
	List(ctx context.Context, status domain.HazardSuggestionStatus) ([]domain.HazardSourceSuggestion, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.HazardSourceSuggestion, error)
	// FindByURLKey returns the suggestion for a source, or nil.
	FindByURLKey(ctx context.Context, country string, hazardType domain.HazardType, urlKey string) (*domain.HazardSourceSuggestion, error)
	// Decide moves a proposed suggestion to status and stamps decided_at. A
	// suggestion that is not (or no longer) proposed is a NotFoundError.
	Decide(ctx context.Context, id uuid.UUID, status domain.HazardSuggestionStatus, decidedAt time.Time) error
}

// HazardSuggestionRepo is the pgx-backed implementation of
// HazardSuggestionRepository.
type HazardSuggestionRepo struct {
	db db.Querier
}

// NewHazardSuggestionRepo constructs a HazardSuggestionRepo wired to q.
func NewHazardSuggestionRepo(q db.Querier) *HazardSuggestionRepo {
	return &HazardSuggestionRepo{db: q}
}

const suggestionColumns = `id, country, hazard_type, scope, title, publisher, url, license, rationale, status, created_at, decided_at`

// Create inserts a suggestion.
func (repo *HazardSuggestionRepo) Create(ctx context.Context, suggestion *domain.HazardSourceSuggestion, urlKey string) error {
	const query = `
INSERT INTO hazard_source_suggestion (id, country, hazard_type, scope, title, publisher, url, url_key, license, rationale, status, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
`
	_, err := repo.db.Exec(ctx, query,
		suggestion.ID, suggestion.Country, suggestion.HazardType, suggestion.Scope, suggestion.Title,
		suggestion.Publisher, suggestion.URL, urlKey, suggestion.License, suggestion.Rationale,
		suggestion.Status, suggestion.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("hazard suggestion create: %w", err)
	}
	return nil
}

// List returns suggestions, newest first.
func (repo *HazardSuggestionRepo) List(ctx context.Context, status domain.HazardSuggestionStatus) ([]domain.HazardSourceSuggestion, error) {
	const query = `
SELECT ` + suggestionColumns + `
FROM hazard_source_suggestion
WHERE $1 = '' OR status = $1
ORDER BY created_at DESC, id
`
	rows, err := repo.db.Query(ctx, query, string(status))
	if err != nil {
		return nil, fmt.Errorf("hazard suggestion list: %w", err)
	}
	defer rows.Close()

	suggestions := make([]domain.HazardSourceSuggestion, 0)
	for rows.Next() {
		suggestion, scanError := scanSuggestion(rows)
		if scanError != nil {
			return nil, fmt.Errorf("hazard suggestion list scan: %w", scanError)
		}
		suggestions = append(suggestions, suggestion)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("hazard suggestion list rows: %w", rowsError)
	}
	return suggestions, nil
}

// GetByID returns one suggestion.
func (repo *HazardSuggestionRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.HazardSourceSuggestion, error) {
	const query = `SELECT ` + suggestionColumns + ` FROM hazard_source_suggestion WHERE id = $1`
	suggestion, err := scanSuggestion(repo.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "hazard source suggestion", ID: id.String()}
		}
		return nil, fmt.Errorf("hazard suggestion get: %w", err)
	}
	return &suggestion, nil
}

// FindByURLKey returns the suggestion for a source, or nil if there is none.
func (repo *HazardSuggestionRepo) FindByURLKey(
	ctx context.Context,
	country string,
	hazardType domain.HazardType,
	urlKey string,
) (*domain.HazardSourceSuggestion, error) {
	const query = `
SELECT ` + suggestionColumns + `
FROM hazard_source_suggestion
WHERE country = $1 AND hazard_type = $2 AND url_key = $3
`
	suggestion, err := scanSuggestion(repo.db.QueryRow(ctx, query, country, hazardType, urlKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil // "no such suggestion" is a normal answer here, not an error
		}
		return nil, fmt.Errorf("hazard suggestion find: %w", err)
	}
	return &suggestion, nil
}

// Decide moves a proposed suggestion to status.
func (repo *HazardSuggestionRepo) Decide(ctx context.Context, id uuid.UUID, status domain.HazardSuggestionStatus, decidedAt time.Time) error {
	const query = `
UPDATE hazard_source_suggestion
SET status = $2, decided_at = $3
WHERE id = $1 AND status = 'proposed'
`
	tag, err := repo.db.Exec(ctx, query, id, status, decidedAt)
	if err != nil {
		return fmt.Errorf("hazard suggestion decide: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "proposed hazard source suggestion", ID: id.String()}
	}
	return nil
}

func scanSuggestion(scanner interface {
	Scan(dest ...any) error
},
) (domain.HazardSourceSuggestion, error) {
	var suggestion domain.HazardSourceSuggestion
	err := scanner.Scan(
		&suggestion.ID, &suggestion.Country, &suggestion.HazardType, &suggestion.Scope, &suggestion.Title,
		&suggestion.Publisher, &suggestion.URL, &suggestion.License, &suggestion.Rationale,
		&suggestion.Status, &suggestion.CreatedAt, &suggestion.DecidedAt,
	)
	return suggestion, err
}
