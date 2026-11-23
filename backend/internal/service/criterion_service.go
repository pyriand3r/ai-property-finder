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
//
// A criterion is "proposed" or "active" (docs/adr/0024). Only active ones
// feed evaluation and scores, so changing or deleting a proposed criterion
// triggers neither side effect; approving one does, like adding it.
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

// Create validates and inserts a new active criterion (the plain form and
// the REST API — the user's own write needs no approval, ADR 0001),
// appended to the end of the profile's ordering, and flags every listing in
// the profile for re-evaluation.
func (service *CriterionService) Create(
	ctx context.Context,
	profileID uuid.UUID,
	name, description string,
	weight int,
	isSubjective bool,
) (*domain.Criterion, error) {
	return service.create(ctx, profileID, name, description, weight, isSubjective, domain.CriterionStatusActive)
}

// Propose validates and inserts a new criterion with status "proposed".
// A proposal is ignored by evaluation and scoring until Approve, so no
// listing is flagged for re-evaluation (docs/adr/0024).
func (service *CriterionService) Propose(
	ctx context.Context,
	profileID uuid.UUID,
	name, description string,
	weight int,
	isSubjective bool,
) (*domain.Criterion, error) {
	return service.create(ctx, profileID, name, description, weight, isSubjective, domain.CriterionStatusProposed)
}

func (service *CriterionService) create(
	ctx context.Context,
	profileID uuid.UUID,
	name, description string,
	weight int,
	isSubjective bool,
	status domain.CriterionStatus,
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
	if conflictError := ensureCriterionNameFree(ctx, criterionRepo, profileID, name, uuid.Nil); conflictError != nil {
		return nil, conflictError
	}
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
		Status:       status,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	txError := db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		if createError := repository.NewCriterionRepo(tx).Create(ctx, criterion); createError != nil {
			return createError
		}
		if status == domain.CriterionStatusProposed {
			return nil
		}
		return repository.NewListingRepo(tx).FlagNeedsReevaluation(ctx, profileID)
	})
	if txError != nil {
		return nil, txError
	}
	return criterion, nil
}

// Approve turns a proposed criterion into an active one and flags every
// listing in the profile for re-evaluation, exactly as adding an active
// criterion does. Approving an already active criterion is a 409.
func (service *CriterionService) Approve(ctx context.Context, id uuid.UUID) (*domain.Criterion, error) {
	criterion, err := repository.NewCriterionRepo(service.pool).GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if criterion.Status != domain.CriterionStatusProposed {
		return nil, &domain.ConflictError{Message: "Kriterium ist bereits aktiv: " + criterion.Name}
	}

	criterion.Status = domain.CriterionStatusActive
	criterion.UpdatedAt = time.Now().UTC()
	txError := db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		if activateError := repository.NewCriterionRepo(tx).Activate(ctx, id, criterion.UpdatedAt); activateError != nil {
			return activateError
		}
		return repository.NewListingRepo(tx).FlagNeedsReevaluation(ctx, criterion.ProfileID)
	})
	if txError != nil {
		return nil, txError
	}
	return criterion, nil
}

// UpdateProposed is Update restricted to proposed criteria — the only
// change an MCP client may make (docs/adr/0024). An active criterion is a
// 409 and nothing is written.
func (service *CriterionService) UpdateProposed(
	ctx context.Context,
	id uuid.UUID,
	name, description *string,
	weight *int,
	isSubjective *bool,
) (*domain.Criterion, error) {
	criterion, err := repository.NewCriterionRepo(service.pool).GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if criterion.Status != domain.CriterionStatusProposed {
		return nil, &domain.ConflictError{Message: "Aktive Kriterien können nur im Chat oder im Formular geändert werden: " + criterion.Name}
	}
	return service.Update(ctx, id, name, description, weight, isSubjective)
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
			if conflictError := ensureCriterionNameFree(ctx, criterionRepo, criterion.ProfileID, trimmed, criterion.ID); conflictError != nil {
				return nil, conflictError
			}
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
		case criterion.Status == domain.CriterionStatusProposed:
			return nil
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
// re-evaluation (removal is not a weight-only change). Deleting a proposed
// criterion changes nothing evaluation sees, so it flags nothing.
func (service *CriterionService) Delete(ctx context.Context, id uuid.UUID) error {
	criterion, err := repository.NewCriterionRepo(service.pool).GetByID(ctx, id)
	if err != nil {
		return err
	}

	return db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		if deleteError := repository.NewCriterionRepo(tx).Delete(ctx, id); deleteError != nil {
			return deleteError
		}
		if criterion.Status == domain.CriterionStatusProposed {
			return nil
		}
		return repository.NewListingRepo(tx).FlagNeedsReevaluation(ctx, criterion.ProfileID)
	})
}

// ensureCriterionNameFree enforces unique criterion names per profile,
// case-insensitive, across proposed and active criteria alike (docs/adr/0024).
// exceptID is the criterion being renamed (uuid.Nil for a new one).
func ensureCriterionNameFree(ctx context.Context, criterionRepo repository.CriterionRepository, profileID uuid.UUID, name string, exceptID uuid.UUID) error {
	existing, err := criterionRepo.ListByProfile(ctx, profileID)
	if err != nil {
		return err
	}
	for _, criterion := range existing {
		if criterion.ID != exceptID && strings.EqualFold(criterion.Name, name) {
			return &domain.ConflictError{Message: "Kriterium existiert bereits: " + name}
		}
	}
	return nil
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
