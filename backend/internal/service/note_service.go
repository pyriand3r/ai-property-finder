package service

import (
	"context"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NoteService orchestrates ListingNote CRUD (spec 01-11, RF.4). Notes are the
// user's own freeform running notes about a Listing — distinct from
// Listing.SellerNotes (a single field about the seller/contact).
type NoteService struct {
	pool *pgxpool.Pool
}

// NewNoteService constructs a NoteService wired to pool.
func NewNoteService(pool *pgxpool.Pool) *NoteService {
	return &NoteService{pool: pool}
}

// List returns a listing's notes, newest first.
func (service *NoteService) List(ctx context.Context, listingID uuid.UUID) ([]domain.ListingNote, error) {
	return repository.NewNoteRepo(service.pool).ListByListing(ctx, listingID)
}

// Create validates content is non-empty, verifies the listing exists, and
// inserts a new note.
func (service *NoteService) Create(ctx context.Context, listingID uuid.UUID, content string) (*domain.ListingNote, error) {
	content = strings.TrimSpace(content)
	if validationError := validateNoteContent(content); validationError != nil {
		return nil, validationError
	}
	if _, err := repository.NewListingRepo(service.pool).GetByID(ctx, listingID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	note := &domain.ListingNote{
		ID:        uuid.New(),
		ListingID: listingID,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repository.NewNoteRepo(service.pool).Create(ctx, note); err != nil {
		return nil, err
	}
	return note, nil
}

// Update applies an in-place content edit.
func (service *NoteService) Update(ctx context.Context, id uuid.UUID, content string) (*domain.ListingNote, error) {
	content = strings.TrimSpace(content)
	if validationError := validateNoteContent(content); validationError != nil {
		return nil, validationError
	}
	return repository.NewNoteRepo(service.pool).Update(ctx, id, content)
}

// Delete removes a note.
func (service *NoteService) Delete(ctx context.Context, id uuid.UUID) error {
	return repository.NewNoteRepo(service.pool).Delete(ctx, id)
}

func validateNoteContent(content string) error {
	if content == "" {
		return &domain.ValidationError{Field: "content", Message: "Inhalt ist erforderlich"}
	}
	return nil
}
