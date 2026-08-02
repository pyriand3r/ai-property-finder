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
