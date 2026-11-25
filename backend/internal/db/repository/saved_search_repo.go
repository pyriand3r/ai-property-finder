package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"propertyfinder/internal/db"
	"propertyfinder/internal/domain"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SavedSearchRepository persists saved portal searches and the outcome of
// their runs (docs/adr/0029).
type SavedSearchRepository interface {
	// Create inserts a saved search. The caller sets every field; a second
	// search with the same name in the profile violates the unique constraint.
	Create(ctx context.Context, search *domain.SavedSearch) error
	List(ctx context.Context, profileID uuid.UUID) ([]domain.SavedSearch, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error)
	// Update writes the editable fields, the schedule and updated_at.
	Update(ctx context.Context, search *domain.SavedSearch) error
	Delete(ctx context.Context, id uuid.UUID) error
	// ListDue returns enabled searches whose next run is due at now and that
	// are not running.
	ListDue(ctx context.Context, now time.Time) ([]domain.SavedSearch, error)
	// Claim marks a search running and moves its next run one interval past
	// now (nil while disabled). It reports false when the search is already
	// running, or, unless force is set, no longer due: the run was taken by
	// someone else first.
	Claim(ctx context.Context, id uuid.UUID, now time.Time, force bool) (bool, error)
	// Finish writes the outcome of a run.
	Finish(ctx context.Context, id uuid.UUID, outcome domain.SavedSearchOutcome) error
	// SeenLinks returns the matchKeys of the links earlier runs reported.
	SeenLinks(ctx context.Context, id uuid.UUID) ([]string, error)
	// FailRunning sets every search left running (the process stopped in the
	// middle of a run) to failed and returns how many there were.
	FailRunning(ctx context.Context, message string) (int64, error)
}

// SavedSearchRepo is the pgx-backed implementation of SavedSearchRepository.
type SavedSearchRepo struct {
	db db.Querier
}

// NewSavedSearchRepo constructs a SavedSearchRepo wired to q.
func NewSavedSearchRepo(q db.Querier) *SavedSearchRepo {
	return &SavedSearchRepo{db: q}
}

const savedSearchColumns = `id, profile_id, name, url, page_param, max_pages, link_contains, interval_hours, enabled, next_run_at,
import_new, max_import, last_status, last_run_at, last_task_id, last_found, last_new, last_imported, last_summary, last_error,
created_at, updated_at`

// Create inserts a saved search.
func (repo *SavedSearchRepo) Create(ctx context.Context, search *domain.SavedSearch) error {
	const query = `
INSERT INTO saved_search (id, profile_id, name, url, page_param, max_pages, link_contains, interval_hours, enabled, next_run_at,
                          import_new, max_import, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
`
	_, err := repo.db.Exec(ctx, query,
		search.ID, search.ProfileID, search.Name, search.URL, search.PageParam, search.MaxPages, search.LinkContains,
		search.IntervalHours, search.Enabled, search.NextRunAt, search.ImportNew, search.MaxImport,
		search.CreatedAt, search.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("saved search create: %w", err)
	}
	return nil
}

// List returns the saved searches of a profile in creation order.
func (repo *SavedSearchRepo) List(ctx context.Context, profileID uuid.UUID) ([]domain.SavedSearch, error) {
	const query = `SELECT ` + savedSearchColumns + ` FROM saved_search WHERE profile_id = $1 ORDER BY created_at, id`
	return repo.queryAll(ctx, "list", query, profileID)
}

// GetByID returns one saved search.
func (repo *SavedSearchRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
	const query = `SELECT ` + savedSearchColumns + ` FROM saved_search WHERE id = $1`
	search, err := scanSavedSearch(repo.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "saved_search", ID: id.String()}
		}
		return nil, fmt.Errorf("saved search get: %w", err)
	}
	return &search, nil
}

// Update writes the editable fields, the schedule and updated_at.
func (repo *SavedSearchRepo) Update(ctx context.Context, search *domain.SavedSearch) error {
	const query = `
UPDATE saved_search
SET name = $2, url = $3, page_param = $4, max_pages = $5, link_contains = $6, interval_hours = $7,
    enabled = $8, next_run_at = $9, import_new = $10, max_import = $11, updated_at = $12
WHERE id = $1
`
	tag, err := repo.db.Exec(ctx, query,
		search.ID, search.Name, search.URL, search.PageParam, search.MaxPages, search.LinkContains,
		search.IntervalHours, search.Enabled, search.NextRunAt, search.ImportNew, search.MaxImport, search.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("saved search update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "saved_search", ID: search.ID.String()}
	}
	return nil
}

// Delete removes a saved search.
func (repo *SavedSearchRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := repo.db.Exec(ctx, `DELETE FROM saved_search WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("saved search delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "saved_search", ID: id.String()}
	}
	return nil
}

// ListDue returns the searches to run at now, oldest due first.
func (repo *SavedSearchRepo) ListDue(ctx context.Context, now time.Time) ([]domain.SavedSearch, error) {
	const query = `
SELECT ` + savedSearchColumns + `
FROM saved_search
WHERE enabled AND next_run_at <= $1 AND last_status <> 'running'
ORDER BY next_run_at, id
`
	return repo.queryAll(ctx, "list due", query, now)
}

// Claim marks a search running; see SavedSearchRepository.Claim.
func (repo *SavedSearchRepo) Claim(ctx context.Context, id uuid.UUID, now time.Time, force bool) (bool, error) {
	const query = `
UPDATE saved_search
SET last_status = 'running', last_run_at = $2, last_error = NULL, updated_at = $2,
    next_run_at = CASE WHEN enabled THEN $2::timestamptz + make_interval(hours => interval_hours) END
WHERE id = $1 AND last_status <> 'running' AND ($3 OR (enabled AND next_run_at <= $2))
`
	tag, err := repo.db.Exec(ctx, query, id, now, force)
	if err != nil {
		return false, fmt.Errorf("saved search claim: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// Finish writes the outcome of a run.
func (repo *SavedSearchRepo) Finish(ctx context.Context, id uuid.UUID, outcome domain.SavedSearchOutcome) error {
	// nil SeenLinks (a failed run) leaves the stored list as it is.
	var encoded []byte
	if outcome.SeenLinks != nil {
		var marshalError error
		encoded, marshalError = json.Marshal(outcome.SeenLinks)
		if marshalError != nil {
			return fmt.Errorf("saved search finish: encode seen links: %w", marshalError)
		}
	}
	const query = `
UPDATE saved_search
SET last_status = $2, last_task_id = $3, last_found = $4, last_new = $5, last_imported = $6,
    last_summary = $7, last_error = $8, seen_links = COALESCE($9::jsonb, seen_links), updated_at = NOW()
WHERE id = $1
`
	tag, execError := repo.db.Exec(ctx, query,
		id, outcome.Status, outcome.TaskID, outcome.Found, outcome.New, outcome.Imported,
		outcome.Summary, outcome.Error, encoded,
	)
	if execError != nil {
		return fmt.Errorf("saved search finish: %w", execError)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "saved_search", ID: id.String()}
	}
	return nil
}

// SeenLinks returns the links earlier runs reported.
func (repo *SavedSearchRepo) SeenLinks(ctx context.Context, id uuid.UUID) ([]string, error) {
	var raw []byte
	err := repo.db.QueryRow(ctx, `SELECT seen_links FROM saved_search WHERE id = $1`, id).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "saved_search", ID: id.String()}
		}
		return nil, fmt.Errorf("saved search seen links: %w", err)
	}
	var seen []string
	if unmarshalError := json.Unmarshal(raw, &seen); unmarshalError != nil {
		return nil, fmt.Errorf("saved search seen links: decode: %w", unmarshalError)
	}
	return seen, nil
}

// FailRunning sets every search left running to failed.
func (repo *SavedSearchRepo) FailRunning(ctx context.Context, message string) (int64, error) {
	const query = `UPDATE saved_search SET last_status = 'failed', last_error = $1, updated_at = NOW() WHERE last_status = 'running'`
	tag, err := repo.db.Exec(ctx, query, message)
	if err != nil {
		return 0, fmt.Errorf("saved search fail running: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (repo *SavedSearchRepo) queryAll(ctx context.Context, op, query string, args ...any) ([]domain.SavedSearch, error) {
	rows, err := repo.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("saved search %s: %w", op, err)
	}
	defer rows.Close()

	searches := make([]domain.SavedSearch, 0)
	for rows.Next() {
		search, scanError := scanSavedSearch(rows)
		if scanError != nil {
			return nil, fmt.Errorf("saved search %s scan: %w", op, scanError)
		}
		searches = append(searches, search)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("saved search %s rows: %w", op, rowsError)
	}
	return searches, nil
}

func scanSavedSearch(row pgx.Row) (domain.SavedSearch, error) {
	var search domain.SavedSearch
	err := row.Scan(
		&search.ID, &search.ProfileID, &search.Name, &search.URL, &search.PageParam, &search.MaxPages, &search.LinkContains,
		&search.IntervalHours, &search.Enabled, &search.NextRunAt, &search.ImportNew, &search.MaxImport,
		&search.LastStatus, &search.LastRunAt, &search.LastTaskID, &search.LastFound, &search.LastNew, &search.LastImported,
		&search.LastSummary, &search.LastError, &search.CreatedAt, &search.UpdatedAt,
	)
	return search, err
}
