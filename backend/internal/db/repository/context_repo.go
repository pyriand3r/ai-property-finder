package repository

import (
	"context"
	"errors"
	"fmt"
	"propertyfinder/internal/db"
	"propertyfinder/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ContextRepository persists Context Files and their revision history.
type ContextRepository interface {
	GetGlobal(ctx context.Context) (*domain.Context, error)
	GetByProfileID(ctx context.Context, profileID uuid.UUID) (*domain.Context, error)
	// CreateProfileContext inserts the empty Profile Context row a Search
	// Profile owns from creation onward (spec 00-02: exactly one per
	// profile).
	CreateProfileContext(ctx context.Context, id, profileID uuid.UUID) error
	// UpdateContent overwrites content_md and bumps updated_at.
	UpdateContent(ctx context.Context, contextID uuid.UUID, contentMD string) error
	CreateRevision(ctx context.Context, revision *domain.ContextRevision) error
	// ListRevisions returns a context's revisions, newest first.
	ListRevisions(ctx context.Context, contextID uuid.UUID) ([]domain.ContextRevision, error)
}

// ContextRepo is the pgx-backed implementation of ContextRepository.
type ContextRepo struct {
	db db.Querier
}

// NewContextRepo constructs a ContextRepo wired to q — either the pool or a
// transaction.
func NewContextRepo(q db.Querier) *ContextRepo {
	return &ContextRepo{db: q}
}

// GetGlobal returns the single app-wide Global Context row, seeded by
// migration 000001 and never absent.
func (repo *ContextRepo) GetGlobal(ctx context.Context) (*domain.Context, error) {
	const query = `
SELECT id, scope, profile_id, content_md, created_at, updated_at
FROM context
WHERE scope = 'global'
`
	row := repo.db.QueryRow(ctx, query)
	contextRow, err := scanContextRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "global context", ID: ""}
		}
		return nil, fmt.Errorf("context get global: %w", err)
	}
	return &contextRow, nil
}

// GetByProfileID returns the Profile Context owned by profileID.
func (repo *ContextRepo) GetByProfileID(ctx context.Context, profileID uuid.UUID) (*domain.Context, error) {
	const query = `
SELECT id, scope, profile_id, content_md, created_at, updated_at
FROM context
WHERE scope = 'profile' AND profile_id = $1
`
	row := repo.db.QueryRow(ctx, query, profileID)
	contextRow, err := scanContextRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "profile context", ID: profileID.String()}
		}
		return nil, fmt.Errorf("context get by profile: %w", err)
	}
	return &contextRow, nil
}

// CreateProfileContext inserts the empty Profile Context row for a newly
// created Search Profile.
func (repo *ContextRepo) CreateProfileContext(ctx context.Context, id, profileID uuid.UUID) error {
	const query = `
INSERT INTO context (id, scope, profile_id, content_md, created_at, updated_at)
VALUES ($1, 'profile', $2, '', NOW(), NOW())
`
	if _, err := repo.db.Exec(ctx, query, id, profileID); err != nil {
		return fmt.Errorf("context create profile context: %w", err)
	}
	return nil
}

// UpdateContent overwrites content_md and bumps updated_at.
func (repo *ContextRepo) UpdateContent(ctx context.Context, contextID uuid.UUID, contentMD string) error {
	const query = `
UPDATE context
SET content_md = $2, updated_at = NOW()
WHERE id = $1
`
	tag, err := repo.db.Exec(ctx, query, contextID, contentMD)
	if err != nil {
		return fmt.Errorf("context update content: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "context", ID: contextID.String()}
	}
	return nil
}

// CreateRevision inserts an append-only revision row. Callers only call this
// when content_md actually changed (spec 00-02).
func (repo *ContextRepo) CreateRevision(ctx context.Context, revision *domain.ContextRevision) error {
	const query = `
INSERT INTO context_revision (id, context_id, content_md, changed_by, chat_message_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
`
	_, err := repo.db.Exec(ctx, query,
		revision.ID,
		revision.ContextID,
		revision.ContentMD,
		revision.ChangedBy,
		revision.ChatMessageID,
		revision.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("context create revision: %w", err)
	}
	return nil
}

// ListRevisions returns a context's revisions, newest first.
func (repo *ContextRepo) ListRevisions(ctx context.Context, contextID uuid.UUID) ([]domain.ContextRevision, error) {
	const query = `
SELECT id, context_id, content_md, changed_by, chat_message_id, created_at
FROM context_revision
WHERE context_id = $1
ORDER BY created_at DESC
`
	rows, err := repo.db.Query(ctx, query, contextID)
	if err != nil {
		return nil, fmt.Errorf("context list revisions: %w", err)
	}
	defer rows.Close()

	revisions := make([]domain.ContextRevision, 0)
	for rows.Next() {
		var revision domain.ContextRevision
		if scanError := rows.Scan(
			&revision.ID,
			&revision.ContextID,
			&revision.ContentMD,
			&revision.ChangedBy,
			&revision.ChatMessageID,
			&revision.CreatedAt,
		); scanError != nil {
			return nil, fmt.Errorf("context revision scan: %w", scanError)
		}
		revisions = append(revisions, revision)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("context revision rows: %w", rowsError)
	}
	return revisions, nil
}

func scanContextRow(scanner interface {
	Scan(dest ...any) error
},
) (domain.Context, error) {
	var contextRow domain.Context
	err := scanner.Scan(
		&contextRow.ID,
		&contextRow.Scope,
		&contextRow.ProfileID,
		&contextRow.ContentMD,
		&contextRow.CreatedAt,
		&contextRow.UpdatedAt,
	)
	return contextRow, err
}
