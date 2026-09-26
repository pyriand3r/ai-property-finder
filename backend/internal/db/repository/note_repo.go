package repository

import (
	"context"
	"errors"
	"fmt"
	"propertyfinder/internal/db"
	"propertyfinder/internal/domain"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// NoteRepository persists the listing_note aggregate.
type NoteRepository interface {
	Create(ctx context.Context, note *domain.ListingNote) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ListingNote, error)
	ListByListing(ctx context.Context, listingID uuid.UUID) ([]domain.ListingNote, error)
	Update(ctx context.Context, id uuid.UUID, content string) (*domain.ListingNote, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// NoteRepo is the pgx-backed implementation of NoteRepository.
type NoteRepo struct {
	db db.Querier
}

// NewNoteRepo constructs a NoteRepo wired to q — either the pool or a
// transaction.
func NewNoteRepo(q db.Querier) *NoteRepo {
	return &NoteRepo{db: q}
}

const noteColumns = `id, listing_id, content, created_at, updated_at`

func (repo *NoteRepo) Create(ctx context.Context, note *domain.ListingNote) error {
	query := `INSERT INTO listing_note (` + noteColumns + `) VALUES ($1, $2, $3, $4, $5)`
	_, err := repo.db.Exec(ctx, query,
		note.ID,
		note.ListingID,
		note.Content,
		note.CreatedAt,
		note.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("note create: %w", err)
	}
	return nil
}

func (repo *NoteRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.ListingNote, error) {
	query := `SELECT ` + noteColumns + ` FROM listing_note WHERE id = $1`
	row := repo.db.QueryRow(ctx, query, id)
	note, err := scanNoteRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "listing_note", ID: id.String()}
		}
		return nil, fmt.Errorf("note get: %w", err)
	}
	return &note, nil
}

func (repo *NoteRepo) ListByListing(ctx context.Context, listingID uuid.UUID) ([]domain.ListingNote, error) {
	query := `SELECT ` + noteColumns + ` FROM listing_note WHERE listing_id = $1 ORDER BY created_at DESC`
	rows, err := repo.db.Query(ctx, query, listingID)
	if err != nil {
		return nil, fmt.Errorf("note list: %w", err)
	}
	defer rows.Close()

	notes := make([]domain.ListingNote, 0)
	for rows.Next() {
		note, scanError := scanNoteRow(rows)
		if scanError != nil {
			return nil, fmt.Errorf("note list scan: %w", scanError)
		}
		notes = append(notes, note)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("note list rows: %w", rowsError)
	}
	return notes, nil
}

func (repo *NoteRepo) Update(ctx context.Context, id uuid.UUID, content string) (*domain.ListingNote, error) {
	query := `UPDATE listing_note SET content = $2, updated_at = $3 WHERE id = $1 RETURNING ` + noteColumns
	row := repo.db.QueryRow(ctx, query, id, content, time.Now().UTC())
	note, err := scanNoteRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "listing_note", ID: id.String()}
		}
		return nil, fmt.Errorf("note update: %w", err)
	}
	return &note, nil
}

func (repo *NoteRepo) Delete(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM listing_note WHERE id = $1`
	tag, err := repo.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("note delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "listing_note", ID: id.String()}
	}
	return nil
}

func scanNoteRow(scanner interface {
	Scan(dest ...any) error
},
) (domain.ListingNote, error) {
	var note domain.ListingNote
	err := scanner.Scan(
		&note.ID,
		&note.ListingID,
		&note.Content,
		&note.CreatedAt,
		&note.UpdatedAt,
	)
	return note, err
}
