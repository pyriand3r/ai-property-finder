package service

import (
	"context"
	"errors"
	"propertyfinder/internal/ai"
	"propertyfinder/internal/db"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EvaluationResult bundles the updated listing and its fresh grades for the
// POST /api/listings/{id}/evaluate response.
type EvaluationResult struct {
	Listing *domain.Listing
	Grades  []domain.CriterionScore
}

// EvaluationService runs the single-shot evaluate_listing operation (spec
// 01-04 Layer 2.2) — no Agent, no tool use, one LLM call producing Grades
// + a computed Score.
type EvaluationService struct {
	pool     *pgxpool.Pool
	provider ai.Provider
}

// NewEvaluationService constructs an EvaluationService wired to pool and
// provider.
func NewEvaluationService(pool *pgxpool.Pool, provider ai.Provider) *EvaluationService {
	return &EvaluationService{pool: pool, provider: provider}
}

// EvaluateListing grades listingID against its profile's Criteria, replaces
// its listing_criterion_grade rows, recomputes overall_score, and clears
// needs_reevaluation — all in one transaction. Grades are mapped back to
// Criteria by position, never by an ID the model echoes (ADR 0006).
func (service *EvaluationService) EvaluateListing(ctx context.Context, listingID uuid.UUID) (*EvaluationResult, error) {
	listingRepo := repository.NewListingRepo(service.pool)
	listing, err := listingRepo.GetByID(ctx, listingID)
	if err != nil {
		return nil, err
	}

	criteria, err := repository.NewCriterionRepo(service.pool).ListByProfile(ctx, listing.ProfileID)
	if err != nil {
		return nil, err
	}
	if len(criteria) == 0 {
		return nil, &domain.ValidationError{Field: "criteria", Message: "Profil hat keine Kriterien — zuerst Kriterien anlegen"}
	}

	scores, err := service.provider.EvaluateListing(ctx, *listing, criteria)
	if err != nil {
		return nil, mapProviderError(err)
	}

	txError := db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		if replaceError := repository.NewGradeRepo(tx).ReplaceForListing(ctx, listingID, scores); replaceError != nil {
			return replaceError
		}
		if recomputeError := repository.NewListingRepo(tx).RecomputeScores(ctx, listing.ProfileID); recomputeError != nil {
			return recomputeError
		}
		return repository.NewListingRepo(tx).ClearNeedsReevaluation(ctx, listingID)
	})
	if txError != nil {
		return nil, txError
	}

	updated, err := listingRepo.GetByID(ctx, listingID)
	if err != nil {
		return nil, err
	}
	return &EvaluationResult{Listing: updated, Grades: scores}, nil
}

// mapProviderError translates ai package sentinel errors into domain errors
// the API layer already knows how to render. ErrNotConfigured is a client-
// actionable 400 (fix app_config); the rest are transient upstream (LLM)
// failures, mapped to 502 rather than an opaque 500 — the request itself
// was well-formed, retrying later may succeed.
func mapProviderError(err error) error {
	switch {
	case errors.Is(err, ai.ErrNotConfigured):
		return &domain.ValidationError{Field: "llm", Message: "LLM-Provider ist nicht konfiguriert (llm.provider/llm.api_key/llm.model in app_config setzen)"}
	case errors.Is(err, ai.ErrListingTooLong):
		return &domain.UpstreamError{Message: "Inserat ist zu umfangreich für das Kontextfenster des Modells"}
	case errors.Is(err, ai.ErrTimeout):
		return &domain.UpstreamError{Message: "LLM-Anfrage hat das Zeitlimit überschritten"}
	case errors.Is(err, ai.ErrParse):
		return &domain.UpstreamError{Message: "LLM-Antwort konnte nicht verarbeitet werden"}
	default:
		return err
	}
}
