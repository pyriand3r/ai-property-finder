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

// CriterionRepository persists the criterion aggregate.
type CriterionRepository interface {
	ListByProfile(ctx context.Context, profileID uuid.UUID) ([]domain.Criterion, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Criterion, error)
	Create(ctx context.Context, criterion *domain.Criterion) error
	Update(ctx context.Context, criterion *domain.Criterion) error
	Delete(ctx context.Context, id uuid.UUID) error
	// CountByProfile is used to assign a new criterion's sort_order
	// (append at the end).
	CountByProfile(ctx context.Context, profileID uuid.UUID) (int, error)
}

// CriterionRepo is the pgx-backed implementation of CriterionRepository.
type CriterionRepo struct {
	db db.Querier
}

// NewCriterionRepo constructs a CriterionRepo wired to q — either the pool
// or a transaction.
func NewCriterionRepo(q db.Querier) *CriterionRepo {
	return &CriterionRepo{db: q}
}

// ListByProfile returns a profile's criteria ordered by sort_order.
func (repo *CriterionRepo) ListByProfile(ctx context.Context, profileID uuid.UUID) ([]domain.Criterion, error) {
	const query = `
SELECT id, profile_id, name, description, weight, is_subjective, sort_order, created_at, updated_at
FROM criterion
WHERE profile_id = $1
ORDER BY sort_order
`
	rows, err := repo.db.Query(ctx, query, profileID)
	if err != nil {
		return nil, fmt.Errorf("criterion list: %w", err)
	}
	defer rows.Close()

	criteria := make([]domain.Criterion, 0)
	for rows.Next() {
		criterion, scanError := scanCriterionRow(rows)
		if scanError != nil {
			return nil, fmt.Errorf("criterion list scan: %w", scanError)
		}
		criteria = append(criteria, criterion)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("criterion list rows: %w", rowsError)
	}
	return criteria, nil
}

// GetByID returns a single criterion by its UUID.
func (repo *CriterionRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Criterion, error) {
	const query = `
SELECT id, profile_id, name, description, weight, is_subjective, sort_order, created_at, updated_at
FROM criterion
WHERE id = $1
`
	row := repo.db.QueryRow(ctx, query, id)
	criterion, err := scanCriterionRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "criterion", ID: id.String()}
		}
		return nil, fmt.Errorf("criterion get: %w", err)
	}
	return &criterion, nil
}

// Create inserts a new criterion. The caller generates the UUID, sort_order,
// and timestamps.
func (repo *CriterionRepo) Create(ctx context.Context, criterion *domain.Criterion) error {
	const query = `
INSERT INTO criterion (id, profile_id, name, description, weight, is_subjective, sort_order, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`
	_, err := repo.db.Exec(ctx, query,
		criterion.ID,
		criterion.ProfileID,
		criterion.Name,
		criterion.Description,
		criterion.Weight,
		criterion.IsSubjective,
		criterion.SortOrder,
		criterion.CreatedAt,
		criterion.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("criterion create: %w", err)
	}
	return nil
}

// Update overwrites name, description, weight, and is_subjective, and bumps
// updated_at.
func (repo *CriterionRepo) Update(ctx context.Context, criterion *domain.Criterion) error {
	const query = `
UPDATE criterion
SET name = $2, description = $3, weight = $4, is_subjective = $5, updated_at = $6
WHERE id = $1
`
	tag, err := repo.db.Exec(ctx, query,
		criterion.ID,
		criterion.Name,
		criterion.Description,
		criterion.Weight,
		criterion.IsSubjective,
		criterion.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("criterion update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "criterion", ID: criterion.ID.String()}
	}
	return nil
}

// Delete removes a criterion by its UUID.
func (repo *CriterionRepo) Delete(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM criterion WHERE id = $1`
	tag, err := repo.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("criterion delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "criterion", ID: id.String()}
	}
	return nil
}

// CountByProfile returns how many criteria a profile currently has.
func (repo *CriterionRepo) CountByProfile(ctx context.Context, profileID uuid.UUID) (int, error) {
	const query = `SELECT COUNT(*) FROM criterion WHERE profile_id = $1`
	var count int
	if err := repo.db.QueryRow(ctx, query, profileID).Scan(&count); err != nil {
		return 0, fmt.Errorf("criterion count by profile: %w", err)
	}
	return count, nil
}

func scanCriterionRow(scanner interface {
	Scan(dest ...any) error
},
) (domain.Criterion, error) {
	var criterion domain.Criterion
	err := scanner.Scan(
		&criterion.ID,
		&criterion.ProfileID,
		&criterion.Name,
		&criterion.Description,
		&criterion.Weight,
		&criterion.IsSubjective,
		&criterion.SortOrder,
		&criterion.CreatedAt,
		&criterion.UpdatedAt,
	)
	return criterion, err
}
