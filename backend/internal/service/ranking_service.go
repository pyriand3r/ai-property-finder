package service

import (
	"context"
	"propertyfinder/internal/ai"
	"propertyfinder/internal/db"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RankingResult bundles the updated profile and its rankable listings
// (now carrying fresh Kurzfazit text) for the ranking-refresh response.
type RankingResult struct {
	Profile  *domain.SearchProfile
	Listings []domain.Listing
}

// RankingService runs the single-shot "Rangliste aktualisieren" operation
// (spec 01-05 Layer 5.2) — one isolated LLM call, no Agent, writing
// listing.kurzfazit and search_profile.ranking_summary. Score/sort order
// stays a pure-SQL concern (ListRankable); the LLM only narrates it.
type RankingService struct {
	pool     *pgxpool.Pool
	provider ai.Provider
}

// NewRankingService constructs a RankingService wired to pool and provider.
func NewRankingService(pool *pgxpool.Pool, provider ai.Provider) *RankingService {
	return &RankingService{pool: pool, provider: provider}
}

// Refresh regenerates Kurzfazit for every rankable listing in profileID
// plus the profile's overall ranking_summary, in one LLM call and one
// transaction. A profile with no scored listings yet is a no-op (nothing
// to narrate) rather than a wasted API call.
func (service *RankingService) Refresh(ctx context.Context, profileID uuid.UUID) (*RankingResult, error) {
	profile, err := repository.NewProfileRepo(service.pool).GetByID(ctx, profileID)
	if err != nil {
		return nil, err
	}

	listingRepo := repository.NewListingRepo(service.pool)
	listings, err := listingRepo.ListRankable(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if len(listings) == 0 {
		return &RankingResult{Profile: profile, Listings: listings}, nil
	}

	result, err := service.provider.GenerateRanking(ctx, listings)
	if err != nil {
		return nil, mapProviderError(err)
	}

	txError := db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		txListingRepo := repository.NewListingRepo(tx)
		for i, listing := range listings {
			if setError := txListingRepo.SetKurzfazit(ctx, listing.ID, result.Kurzfazits[i]); setError != nil {
				return setError
			}
			listings[i].Kurzfazit = &result.Kurzfazits[i]
		}
		return repository.NewProfileRepo(tx).SetRankingSummary(ctx, profileID, result.Summary)
	})
	if txError != nil {
		return nil, txError
	}
	profile.RankingSummary = &result.Summary

	return &RankingResult{Profile: profile, Listings: listings}, nil
}
