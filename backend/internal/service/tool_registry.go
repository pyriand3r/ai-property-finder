package service

import (
	"context"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/fetch"
	"propertyfinder/internal/geocode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewToolRegistry builds an agent.Registry scoped to profileID, wired to
// this process's shared pool/dataDir/evaluation service. Factored out of
// AgentService.runTurn (which calls this for every chat turn) so
// internal/mcpserver (docs/adr/0019) can dispatch the identical Tool
// catalog outside a chat session, for an explicit profile_id, with zero
// duplicated wiring.
func NewToolRegistry(pool *pgxpool.Pool, dataDir string, evaluation *EvaluationService, profileID uuid.UUID) *agent.Registry {
	return &agent.Registry{
		ProfileID:     profileID,
		Listings:      repository.NewListingRepo(pool),
		Criteria:      NewCriterionService(pool),
		Images:        repository.NewImageRepo(pool),
		Fetcher:       fetch.NewClient(),
		Downloader:    fetch.NewClient(),
		DataDir:       dataDir,
		Evaluator:     evaluationAdapter{evaluation},
		Geocoder:      geocodeAdapter{},
		HazardSources: NewHazardSourceService(pool),
	}
}

// geocodeAdapter satisfies agent.AddressGeocoder against the
// package-level geocode.Geocode func.
type geocodeAdapter struct{}

func (geocodeAdapter) Geocode(ctx context.Context, address, country string) (*geocode.Result, error) {
	return geocode.Geocode(ctx, address, country)
}

// evaluationAdapter satisfies agent.ListingEvaluator against
// *EvaluationService without internal/agent importing internal/service
// (which would cycle — service already imports agent for AgentService).
type evaluationAdapter struct {
	service *EvaluationService
}

func (a evaluationAdapter) EvaluateListing(ctx context.Context, listingID uuid.UUID) (*domain.Listing, []domain.CriterionScore, error) {
	result, err := a.service.EvaluateListing(ctx, listingID)
	if err != nil {
		return nil, nil, err
	}
	return result.Listing, result.Grades, nil
}
