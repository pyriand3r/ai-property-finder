package repository

import (
	"context"
	"fmt"
	"propertyfinder/internal/db"
	"propertyfinder/internal/domain"

	"github.com/google/uuid"
)

// GradeRepository persists listing_criterion_grade rows.
type GradeRepository interface {
	// ReplaceForListing deletes every existing grade for listingID and
	// inserts scores in its place — a full replace on every Evaluation,
	// never a partial merge (spec 01-04).
	ReplaceForListing(ctx context.Context, listingID uuid.UUID, scores []domain.CriterionScore) error
	// ListByListing returns a listing's current grades joined with their
	// criterion's name, ordered by the criterion's sort_order — the
	// Listing Detail View's grade grid (spec 01-07 Layer 5.3).
	ListByListing(ctx context.Context, listingID uuid.UUID) ([]domain.ListingGrade, error)
}

// GradeRepo is the pgx-backed implementation of GradeRepository.
type GradeRepo struct {
	db db.Querier
}

// NewGradeRepo constructs a GradeRepo wired to q — either the pool or a
// transaction.
func NewGradeRepo(q db.Querier) *GradeRepo {
	return &GradeRepo{db: q}
}

func (repo *GradeRepo) ReplaceForListing(ctx context.Context, listingID uuid.UUID, scores []domain.CriterionScore) error {
	if _, err := repo.db.Exec(ctx, `DELETE FROM listing_criterion_grade WHERE listing_id = $1`, listingID); err != nil {
		return fmt.Errorf("grade replace delete: %w", err)
	}

	const insert = `
INSERT INTO listing_criterion_grade (id, listing_id, criterion_id, grade, reasoning, created_at)
VALUES ($1, $2, $3, $4, $5, NOW())
`
	for _, score := range scores {
		if _, err := repo.db.Exec(ctx, insert, uuid.New(), listingID, score.CriterionID, score.Grade, score.Reasoning); err != nil {
			return fmt.Errorf("grade replace insert: %w", err)
		}
	}
	return nil
}

func (repo *GradeRepo) ListByListing(ctx context.Context, listingID uuid.UUID) ([]domain.ListingGrade, error) {
	const query = `
SELECT c.id, c.name, lcg.grade, lcg.reasoning
FROM listing_criterion_grade lcg
JOIN criterion c ON c.id = lcg.criterion_id
WHERE lcg.listing_id = $1
ORDER BY c.sort_order
`
	rows, err := repo.db.Query(ctx, query, listingID)
	if err != nil {
		return nil, fmt.Errorf("grade list by listing: %w", err)
	}
	defer rows.Close()

	grades := make([]domain.ListingGrade, 0)
	for rows.Next() {
		var grade domain.ListingGrade
		if scanError := rows.Scan(&grade.CriterionID, &grade.CriterionName, &grade.Grade, &grade.Reasoning); scanError != nil {
			return nil, fmt.Errorf("grade list by listing scan: %w", scanError)
		}
		grades = append(grades, grade)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("grade list by listing rows: %w", rowsError)
	}
	return grades, nil
}
