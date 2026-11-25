package service

import (
	"context"
	"fmt"
	"propertyfinder/internal/db"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/hazard"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HazardService syncs the static natural-hazard datasets into the database
// and serves them (docs/adr/0025, spec 01-13). Ratings are region-level
// reference data: nothing here reads or writes a Listing, and no hazard
// value feeds a Listing's Score.
type HazardService struct {
	pool *pgxpool.Pool
}

// NewHazardService constructs a HazardService wired to pool.
func NewHazardService(pool *pgxpool.Pool) *HazardService {
	return &HazardService{pool: pool}
}

// SeedStatic syncs every dataset embedded in the binary into the hazard_*
// tables. It is idempotent: a boot with unchanged data writes no rating
// row. The backend calls it once at startup, right after the migrations,
// and stops if it fails — the tables are a cache of the embedded files, so
// a failed sync means the cache is not trustworthy.
func (service *HazardService) SeedStatic(ctx context.Context) error {
	datasets, err := hazard.Load()
	if err != nil {
		return fmt.Errorf("load hazard datasets: %w", err)
	}
	for _, dataset := range datasets {
		if syncError := service.syncDataset(ctx, dataset); syncError != nil {
			return fmt.Errorf("sync hazard dataset %s: %w", dataset.Country, syncError)
		}
	}
	// The source catalog (docs/adr/0027) must agree with the datasets just
	// synced; a catalog that does not stops the boot like a bad dataset.
	if _, catalogError := loadCatalog(); catalogError != nil {
		return fmt.Errorf("load hazard catalog: %w", catalogError)
	}
	return nil
}

// syncDataset makes the tables mirror one country's dataset, in one
// transaction: upsert what the dataset holds, then delete what it no longer
// holds, so a half-applied update is never visible.
func (service *HazardService) syncDataset(ctx context.Context, dataset hazard.Dataset) error {
	regionKeys := make([]string, len(dataset.Regions))
	for i, region := range dataset.Regions {
		regionKeys[i] = region.Key
	}
	return db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		repo := repository.NewHazardRepo(tx)
		if err := repo.UpsertSources(ctx, dataset.Sources); err != nil {
			return err
		}
		if err := repo.UpsertRegions(ctx, dataset.Regions); err != nil {
			return err
		}
		if err := repo.UpsertRatings(ctx, dataset.Ratings); err != nil {
			return err
		}
		if err := repo.PruneRatings(ctx, dataset.Country, dataset.Ratings); err != nil {
			return err
		}
		if err := repo.PruneRegions(ctx, dataset.Country, regionKeys); err != nil {
			return err
		}
		return repo.PruneSources(ctx)
	})
}

// SearchRegions lists regions ordered by name. country limits the result to
// one country ("" = all); query keeps regions whose name contains it ("" =
// all), ignoring case, accents and hyphens, so "evora" finds "Évora".
func (service *HazardService) SearchRegions(ctx context.Context, country, query string) ([]domain.HazardRegion, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if country != "" && !hazard.ValidCountry(country) {
		return nil, &domain.ValidationError{Field: "country", Message: "country muss ein zweistelliger Ländercode sein, z. B. PT"}
	}
	return repository.NewHazardRepo(service.pool).SearchRegions(ctx, country, hazard.NormalizeName(query))
}

// GetRegionHazards returns one region with all its ratings, each with its
// source, in domain.HazardTypes order. A hazard the datasets have no usable
// data for is missing from Ratings. An unknown key is a NotFoundError.
func (service *HazardService) GetRegionHazards(ctx context.Context, regionKey string) (*domain.RegionHazards, error) {
	regionKey = strings.ToUpper(strings.TrimSpace(regionKey))
	if !hazard.ValidRegionKey(regionKey) {
		return nil, &domain.ValidationError{Field: "regionKey", Message: "regionKey ist ungültig, erwartet z. B. PT-1106"}
	}

	repo := repository.NewHazardRepo(service.pool)
	region, err := repo.GetRegion(ctx, regionKey)
	if err != nil {
		return nil, err
	}
	ratings, err := repo.ListRatingsByRegion(ctx, regionKey)
	if err != nil {
		return nil, err
	}
	order := domain.HazardTypes()
	slices.SortFunc(ratings, func(a, b domain.HazardRatingDetail) int {
		return slices.Index(order, a.HazardType) - slices.Index(order, b.HazardType)
	})
	return &domain.RegionHazards{HazardRegion: *region, Ratings: ratings}, nil
}
