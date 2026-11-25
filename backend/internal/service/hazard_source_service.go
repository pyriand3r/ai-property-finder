package service

import (
	"context"
	"fmt"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/hazard"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Field limits keep a model-written suggestion readable in the UI.
const (
	maxSuggestionShortField = 200
	maxSuggestionRationale  = 2000
)

// loadCatalog parses the embedded catalog once; boot already validated it
// (HazardService.SeedStatic), so a failure here cannot happen in a running
// process.
var loadCatalog = sync.OnceValues(hazard.LoadCatalog)

// HazardSourceService serves the hazard source catalog and the review of
// source suggestions (docs/adr/0027, spec 01-15). A suggestion changes no
// rating and is not ingested: approval only clears it for a release that adds
// the dataset (docs/adr/0025).
type HazardSourceService struct {
	pool *pgxpool.Pool
}

// NewHazardSourceService constructs a HazardSourceService wired to pool.
func NewHazardSourceService(pool *pgxpool.Pool) *HazardSourceService {
	return &HazardSourceService{pool: pool}
}

// Catalog returns every catalog entry, bundled and candidate, in file order.
func (service *HazardSourceService) Catalog() ([]domain.HazardCatalogEntry, error) {
	entries, err := loadCatalog()
	if err != nil {
		return nil, fmt.Errorf("load hazard catalog: %w", err)
	}
	return entries, nil
}

// Suggest stores a new source suggestion with status proposed. It refuses a
// source the catalog already lists (a 409 naming the entry) and one that
// was suggested before, whatever the user decided (a 409 naming the status),
// so the Agent does not nag about a source the user rejected.
func (service *HazardSourceService) Suggest(ctx context.Context, input domain.HazardSourceSuggestionInput) (*domain.HazardSourceSuggestion, error) {
	input.Country = strings.ToUpper(strings.TrimSpace(input.Country))
	input.Scope = strings.TrimSpace(input.Scope)
	input.Title = strings.TrimSpace(input.Title)
	input.Publisher = strings.TrimSpace(input.Publisher)
	input.URL = strings.TrimSpace(input.URL)
	input.License = strings.TrimSpace(input.License)
	input.Rationale = strings.TrimSpace(input.Rationale)
	if validationError := validateSuggestion(input); validationError != nil {
		return nil, validationError
	}
	urlKey := hazard.SourceURLKey(input.URL)

	catalog, err := service.Catalog()
	if err != nil {
		return nil, err
	}
	for _, entry := range catalog {
		if entry.Country == input.Country && entry.HazardType == input.HazardType && hazard.SourceURLKey(entry.URL) == urlKey {
			return nil, &domain.ConflictError{Message: fmt.Sprintf("Die Quelle steht schon im Katalog (%s, Status %s).", entry.ID, entry.Status)}
		}
	}

	repo := repository.NewHazardSuggestionRepo(service.pool)
	existing, err := repo.FindByURLKey(ctx, input.Country, input.HazardType, urlKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, &domain.ConflictError{Message: fmt.Sprintf("Die Quelle wurde schon vorgeschlagen (Status %s).", existing.Status)}
	}

	suggestion := &domain.HazardSourceSuggestion{
		ID:         uuid.New(),
		Country:    input.Country,
		HazardType: input.HazardType,
		Scope:      input.Scope,
		Title:      input.Title,
		Publisher:  input.Publisher,
		URL:        input.URL,
		License:    input.License,
		Rationale:  input.Rationale,
		Status:     domain.HazardSuggestionProposed,
		CreatedAt:  time.Now().UTC(),
	}
	if createError := repo.Create(ctx, suggestion, urlKey); createError != nil {
		return nil, createError
	}
	return suggestion, nil
}

func validateSuggestion(input domain.HazardSourceSuggestionInput) error {
	if !hazard.ValidCountry(input.Country) {
		return &domain.ValidationError{Field: "country", Message: "country muss ein zweistelliger Ländercode sein (z.B. SE, AT, PT)"}
	}
	if !input.HazardType.Valid() {
		return &domain.ValidationError{Field: "hazard_type", Message: "hazard_type muss wildfire, flood, earthquake oder landslide sein"}
	}
	if hazard.SourceURLKey(input.URL) == "" {
		return &domain.ValidationError{Field: "url", Message: "url muss eine http(s)-URL sein"}
	}
	for _, field := range []struct {
		name, value string
		limit       int
		required    bool
	}{
		{"title", input.Title, maxSuggestionShortField, true},
		{"publisher", input.Publisher, maxSuggestionShortField, true},
		{"rationale", input.Rationale, maxSuggestionRationale, true},
		{"scope", input.Scope, maxSuggestionShortField, false},
		{"license", input.License, maxSuggestionShortField, false},
		{"url", input.URL, maxSuggestionRationale, false},
	} {
		if field.required && field.value == "" {
			return &domain.ValidationError{Field: field.name, Message: field.name + " ist erforderlich"}
		}
		if len([]rune(field.value)) > field.limit {
			return &domain.ValidationError{Field: field.name, Message: fmt.Sprintf("%s ist zu lang (höchstens %d Zeichen)", field.name, field.limit)}
		}
	}
	return nil
}

// ListSuggestions returns suggestions, newest first. status "" returns all;
// otherwise it must be a known status.
func (service *HazardSourceService) ListSuggestions(ctx context.Context, status string) ([]domain.HazardSourceSuggestion, error) {
	filter := domain.HazardSuggestionStatus(status)
	switch filter {
	case "", domain.HazardSuggestionProposed, domain.HazardSuggestionApproved, domain.HazardSuggestionRejected:
	default:
		return nil, &domain.ValidationError{Field: "status", Message: "status muss proposed, approved oder rejected sein"}
	}
	return repository.NewHazardSuggestionRepo(service.pool).List(ctx, filter)
}

// Approve clears a proposed suggestion for ingestion.
func (service *HazardSourceService) Approve(ctx context.Context, id uuid.UUID) (*domain.HazardSourceSuggestion, error) {
	return service.decide(ctx, id, domain.HazardSuggestionApproved)
}

// Reject marks a proposed suggestion as unwanted. The row stays, so the same
// source is not suggested again.
func (service *HazardSourceService) Reject(ctx context.Context, id uuid.UUID) (*domain.HazardSourceSuggestion, error) {
	return service.decide(ctx, id, domain.HazardSuggestionRejected)
}

func (service *HazardSourceService) decide(ctx context.Context, id uuid.UUID, status domain.HazardSuggestionStatus) (*domain.HazardSourceSuggestion, error) {
	repo := repository.NewHazardSuggestionRepo(service.pool)
	suggestion, err := repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if suggestion.Status != domain.HazardSuggestionProposed {
		return nil, &domain.ConflictError{Message: fmt.Sprintf("Der Vorschlag ist schon entschieden (Status %s): %s", suggestion.Status, suggestion.Title)}
	}
	decidedAt := time.Now().UTC()
	if decideError := repo.Decide(ctx, id, status, decidedAt); decideError != nil {
		return nil, decideError
	}
	suggestion.Status = status
	suggestion.DecidedAt = &decidedAt
	return suggestion, nil
}
