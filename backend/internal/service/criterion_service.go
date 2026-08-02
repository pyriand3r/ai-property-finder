package service

import (
	"context"
	"propertyfinder/internal/db"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CriterionService orchestrates Criterion CRUD and the two side effects a
// change triggers on the profile's listings (spec 01-04): a weight-only
// change recomputes scores in pure SQL; any other change (add, remove, or a
// name/description/is_subjective edit) flags every listing for
// re-evaluation. No LLM call either way — that only happens when the user
// explicitly triggers re-evaluation (ADR 0015, Layer 2.3).
type CriterionService struct {
	pool *pgxpool.Pool
}

// NewCriterionService constructs a CriterionService wired to pool.
func NewCriterionService(pool *pgxpool.Pool) *CriterionService {
	return &CriterionService{pool: pool}
}

// List returns a profile's criteria ordered by sort_order.
func (service *CriterionService) List(ctx context.Context, profileID uuid.UUID) ([]domain.Criterion, error) {
	return repository.NewCriterionRepo(service.pool).ListByProfile(ctx, profileID)
}

// Create validates and inserts a new criterion, appended to the end of the
// profile's ordering, and flags every listing in the profile for
// re-evaluation.
func (service *CriterionService) Create(
	ctx context.Context,
	profileID uuid.UUID,
	name, description string,
	weight int,
	isSubjective bool,
) (*domain.Criterion, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if validationError := validateCriterionName(name); validationError != nil {
		return nil, validationError
	}
	if validationError := validateCriterionWeight(weight); validationError != nil {
		return nil, validationError
	}

	criterionRepo := repository.NewCriterionRepo(service.pool)
	count, err := criterionRepo.CountByProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	criterion := &domain.Criterion{
		ID:           uuid.New(),
		ProfileID:    profileID,
		Name:         name,
		Description:  description,
		Weight:       weight,
		IsSubjective: isSubjective,
		SortOrder:    count,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	txError := db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		if createError := repository.NewCriterionRepo(tx).Create(ctx, criterion); createError != nil {
			return createError
		}
		return repository.NewListingRepo(tx).FlagNeedsReevaluation(ctx, profileID)
	})
	if txError != nil {
		return nil, txError
	}
	return criterion, nil
}

// Update applies a partial update. When weight is the only field actually
// changing, listing scores are recomputed in pure SQL; when name,
// description, or isSubjective change (with or without weight), every
// listing in the profile is flagged for re-evaluation instead.
func (service *CriterionService) Update(
	ctx context.Context,
	id uuid.UUID,
	name, description *string,
	weight *int,
	isSubjective *bool,
) (*domain.Criterion, error) {
	if name == nil && description == nil && weight == nil && isSubjective == nil {
		return nil, &domain.ValidationError{Field: "body", Message: "Mindestens ein Feld muss angegeben werden"}
	}

	criterionRepo := repository.NewCriterionRepo(service.pool)
	criterion, err := criterionRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	weightChanged := false
	otherFieldChanged := false

	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if validationError := validateCriterionName(trimmed); validationError != nil {
			return nil, validationError
		}
		if trimmed != criterion.Name {
			otherFieldChanged = true
		}
		criterion.Name = trimmed
	}
	if description != nil {
		trimmed := strings.TrimSpace(*description)
		if trimmed != criterion.Description {
			otherFieldChanged = true
		}
		criterion.Description = trimmed
	}
	if isSubjective != nil && *isSubjective != criterion.IsSubjective {
		otherFieldChanged = true
		criterion.IsSubjective = *isSubjective
	}
	if weight != nil {
		if validationError := validateCriterionWeight(*weight); validationError != nil {
			return nil, validationError
		}
		if *weight != criterion.Weight {
			weightChanged = true
		}
		criterion.Weight = *weight
	}

	criterion.UpdatedAt = time.Now().UTC()

	txError := db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		if updateError := repository.NewCriterionRepo(tx).Update(ctx, criterion); updateError != nil {
			return updateError
		}
		listingRepo := repository.NewListingRepo(tx)
		switch {
		case otherFieldChanged:
			return listingRepo.FlagNeedsReevaluation(ctx, criterion.ProfileID)
		case weightChanged:
			return listingRepo.RecomputeScores(ctx, criterion.ProfileID)
		default:
			return nil
		}
	})
	if txError != nil {
		return nil, txError
	}
	return criterion, nil
}

// Delete removes a criterion and flags every listing in its profile for
// re-evaluation (removal is not a weight-only change).
func (service *CriterionService) Delete(ctx context.Context, id uuid.UUID) error {
	criterion, err := repository.NewCriterionRepo(service.pool).GetByID(ctx, id)
	if err != nil {
		return err
	}

	return db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		if deleteError := repository.NewCriterionRepo(tx).Delete(ctx, id); deleteError != nil {
			return deleteError
		}
		return repository.NewListingRepo(tx).FlagNeedsReevaluation(ctx, criterion.ProfileID)
	})
}

func validateCriterionName(name string) error {
	if name == "" {
		return &domain.ValidationError{Field: "name", Message: "Name ist erforderlich"}
	}
	return nil
}

func validateCriterionWeight(weight int) error {
	if weight < 1 || weight > 5 {
		return &domain.ValidationError{Field: "weight", Message: "Gewichtung muss zwischen 1 und 5 liegen"}
	}
	return nil
}
