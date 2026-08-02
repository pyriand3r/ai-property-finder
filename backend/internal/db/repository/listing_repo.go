package repository

import (
	"context"
	"fmt"
	"propertyfinder/internal/db"

	"github.com/google/uuid"
)

// ListingRepository persists the listing aggregate. Layer 1 only needs the
// two Criteria-side-effect write paths (spec 01-04); full Listing CRUD
// lands in Layer 2.
type ListingRepository interface {
	// RecomputeScores recalculates listing.overall_score for every listing
	// in profileID that has at least one Grade, using each criterion's
	// *current* weight. Grade letters map A=100 down to F=0 in steps of 20.
	// Listings with no grades yet are left untouched. Used on a weight-only
	// Criterion change (spec 01-04) — no LLM call, no needs_reevaluation
	// flag.
	RecomputeScores(ctx context.Context, profileID uuid.UUID) error
	// FlagNeedsReevaluation sets needs_reevaluation = true on every listing
	// in profileID. Used on any Criteria change other than weight-only
	// (add, remove, or a name/description/is_subjective edit).
	FlagNeedsReevaluation(ctx context.Context, profileID uuid.UUID) error
}

// ListingRepo is the pgx-backed implementation of ListingRepository.
type ListingRepo struct {
	db db.Querier
}

// NewListingRepo constructs a ListingRepo wired to q — either the pool or a
// transaction.
func NewListingRepo(q db.Querier) *ListingRepo {
	return &ListingRepo{db: q}
}

func (repo *ListingRepo) RecomputeScores(ctx context.Context, profileID uuid.UUID) error {
	const query = `
WITH grade_values AS (
    SELECT id, listing_id,
           CASE grade
               WHEN 'A' THEN 100 WHEN 'B' THEN 80 WHEN 'C' THEN 60
               WHEN 'D' THEN 40 WHEN 'E' THEN 20 WHEN 'F' THEN 0
           END AS value
    FROM listing_criterion_grade
),
scores AS (
    SELECT gv.listing_id,
           SUM(gv.value * c.weight)::numeric / NULLIF(SUM(c.weight), 0) AS score
    FROM grade_values gv
    JOIN listing_criterion_grade lcg ON lcg.id = gv.id
    JOIN criterion c ON c.id = lcg.criterion_id
    WHERE c.profile_id = $1
    GROUP BY gv.listing_id
)
UPDATE listing l
SET overall_score = scores.score, updated_at = NOW()
FROM scores
WHERE l.id = scores.listing_id
`
	if _, err := repo.db.Exec(ctx, query, profileID); err != nil {
		return fmt.Errorf("listing recompute scores: %w", err)
	}
	return nil
}

func (repo *ListingRepo) FlagNeedsReevaluation(ctx context.Context, profileID uuid.UUID) error {
	const query = `UPDATE listing SET needs_reevaluation = true, updated_at = NOW() WHERE profile_id = $1`
	if _, err := repo.db.Exec(ctx, query, profileID); err != nil {
		return fmt.Errorf("listing flag needs_reevaluation: %w", err)
	}
	return nil
}
