// Package service orchestrates repositories into the business operations
// the API layer calls: validation, cross-aggregate transactions, and the
// side effects specified in docs/spec (e.g. Criteria changes flagging
// listings for re-evaluation).
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

const maxProfileNameLength = 200

// ProfileService orchestrates search_profile plus the aggregates a profile
// owns from creation onward (Profile Context, chat session).
type ProfileService struct {
	pool *pgxpool.Pool
}

// NewProfileService constructs a ProfileService wired to pool.
func NewProfileService(pool *pgxpool.Pool) *ProfileService {
	return &ProfileService{pool: pool}
}

// List returns all search profiles.
func (service *ProfileService) List(ctx context.Context) ([]domain.SearchProfile, error) {
	return repository.NewProfileRepo(service.pool).List(ctx)
}

// Get returns a single profile.
func (service *ProfileService) Get(ctx context.Context, id uuid.UUID) (*domain.SearchProfile, error) {
	return repository.NewProfileRepo(service.pool).GetByID(ctx, id)
}

// Create validates input and atomically persists a new Search Profile
// together with the two aggregates it owns from day one: an empty Profile
// Context and an active chat session (spec 01-01 Layer 1.1).
func (service *ProfileService) Create(ctx context.Context, name, description string) (*domain.SearchProfile, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if validationError := validateProfileName(name); validationError != nil {
		return nil, validationError
	}

	profileRepo := repository.NewProfileRepo(service.pool)
	exists, err := profileRepo.ExistsByName(ctx, name, nil)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, &domain.ConflictError{Message: "Ein Profil mit diesem Namen existiert bereits"}
	}

	now := time.Now().UTC()
	profile := &domain.SearchProfile{
		ID:          uuid.New(),
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	txError := db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		if createError := repository.NewProfileRepo(tx).Create(ctx, profile); createError != nil {
			return createError
		}
		if contextError := repository.NewContextRepo(tx).CreateProfileContext(ctx, uuid.New(), profile.ID); contextError != nil {
			return contextError
		}
		return repository.NewChatRepo(tx).CreateSession(ctx, uuid.New(), profile.ID)
	})
	if txError != nil {
		return nil, txError
	}
	return profile, nil
}

// Update applies a partial update to name and/or description.
func (service *ProfileService) Update(ctx context.Context, id uuid.UUID, name, description *string) (*domain.SearchProfile, error) {
	profileRepo := repository.NewProfileRepo(service.pool)
	profile, err := profileRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if validationError := validateProfileName(trimmed); validationError != nil {
			return nil, validationError
		}
		if !strings.EqualFold(trimmed, profile.Name) {
			exists, existsError := profileRepo.ExistsByName(ctx, trimmed, &id)
			if existsError != nil {
				return nil, existsError
			}
			if exists {
				return nil, &domain.ConflictError{Message: "Ein Profil mit diesem Namen existiert bereits"}
			}
		}
		profile.Name = trimmed
	}
	if description != nil {
		profile.Description = strings.TrimSpace(*description)
	}

	profile.UpdatedAt = time.Now().UTC()
	if updateError := profileRepo.Update(ctx, profile); updateError != nil {
		return nil, updateError
	}
	return profile, nil
}

// Archive sets archived_at on a profile, hiding it from the default list
// (spec 01-08).
func (service *ProfileService) Archive(ctx context.Context, id uuid.UUID) (*domain.SearchProfile, error) {
	return repository.NewProfileRepo(service.pool).Archive(ctx, id)
}

// Delete hard-deletes a profile and everything it owns via cascading FKs.
func (service *ProfileService) Delete(ctx context.Context, id uuid.UUID) error {
	return repository.NewProfileRepo(service.pool).Delete(ctx, id)
}

// validateProfileName checks the name constraints: required, 1-200 chars.
func validateProfileName(name string) error {
	if name == "" {
		return &domain.ValidationError{Field: "name", Message: "Name ist erforderlich"}
	}
	if len(name) > maxProfileNameLength {
		return &domain.ValidationError{Field: "name", Message: "Name darf maximal 200 Zeichen lang sein"}
	}
	return nil
}
