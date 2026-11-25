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

// TaskRepository persists background task progress. Deliberately minimal —
// there is no queue/dequeue here (spec 01-04 Layer 2.3 runs a single
// sequential goroutine per request, not a polling worker); it only needs to
// create a row, update its counters, and mark it finished. A task that must
// survive a restart (docs/adr/0028) also keeps resumable state in the
// payload column and is found again with ListRunning.
type TaskRepository interface {
	Create(ctx context.Context, task *domain.Task) error
	CreateWithPayload(ctx context.Context, task *domain.Task, payload []byte) error
	SaveProgress(ctx context.Context, taskID uuid.UUID, done, failedCount int, payload []byte) error
	GetPayload(ctx context.Context, taskID uuid.UUID) ([]byte, error)
	ListRunning(ctx context.Context, taskType domain.TaskType) ([]domain.Task, error)
	UpdateProgress(ctx context.Context, taskID uuid.UUID, done, failedCount int) error
	Complete(ctx context.Context, taskID uuid.UUID, status domain.TaskStatus, errMsg *string) error
	GetByID(ctx context.Context, taskID uuid.UUID) (*domain.Task, error)
}

// TaskRepo is the pgx-backed implementation of TaskRepository.
type TaskRepo struct {
	db db.Querier
}

// NewTaskRepo constructs a TaskRepo wired to q — either the pool or a
// transaction.
func NewTaskRepo(q db.Querier) *TaskRepo {
	return &TaskRepo{db: q}
}

const taskColumns = `id, type, status, profile_id, listing_id, total, done, failed_count, error, created_at, started_at, finished_at, updated_at`

// Create inserts a new task row. The caller sets ID, Type, Status (normally
// TaskStatusRunning), ProfileID, Total, and CreatedAt/StartedAt/UpdatedAt.
func (repo *TaskRepo) Create(ctx context.Context, task *domain.Task) error {
	return repo.CreateWithPayload(ctx, task, []byte("{}"))
}

// CreateWithPayload is Create plus the task's resumable state (a JSON
// object; the caller owns its shape).
func (repo *TaskRepo) CreateWithPayload(ctx context.Context, task *domain.Task, payload []byte) error {
	query := `INSERT INTO task (` + taskColumns + `, payload) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`
	_, err := repo.db.Exec(ctx, query,
		task.ID,
		task.Type,
		task.Status,
		task.ProfileID,
		task.ListingID,
		task.Total,
		task.Done,
		task.FailedCount,
		task.Error,
		task.CreatedAt,
		task.StartedAt,
		task.FinishedAt,
		task.UpdatedAt,
		payload,
	)
	if err != nil {
		return fmt.Errorf("task create: %w", err)
	}
	return nil
}

// UpdateProgress writes the done/failed_count counters for a running task.
func (repo *TaskRepo) UpdateProgress(ctx context.Context, taskID uuid.UUID, done, failedCount int) error {
	const query = `UPDATE task SET done = $2, failed_count = $3, updated_at = NOW() WHERE id = $1`
	if _, err := repo.db.Exec(ctx, query, taskID, done, failedCount); err != nil {
		return fmt.Errorf("task update progress: %w", err)
	}
	return nil
}

// SaveProgress writes the counters and the resumable payload in one
// statement, so a restart never sees counters that disagree with the state.
func (repo *TaskRepo) SaveProgress(ctx context.Context, taskID uuid.UUID, done, failedCount int, payload []byte) error {
	const query = `UPDATE task SET done = $2, failed_count = $3, payload = $4, updated_at = NOW() WHERE id = $1`
	if _, err := repo.db.Exec(ctx, query, taskID, done, failedCount, payload); err != nil {
		return fmt.Errorf("task save progress: %w", err)
	}
	return nil
}

// GetPayload returns the task's payload JSON.
func (repo *TaskRepo) GetPayload(ctx context.Context, taskID uuid.UUID) ([]byte, error) {
	const query = `SELECT payload FROM task WHERE id = $1`
	var payload []byte
	if err := repo.db.QueryRow(ctx, query, taskID).Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "task", ID: taskID.String()}
		}
		return nil, fmt.Errorf("task get payload: %w", err)
	}
	return payload, nil
}

// ListRunning returns every task of taskType that is still running, oldest
// first — after a restart these are the tasks whose goroutine died.
func (repo *TaskRepo) ListRunning(ctx context.Context, taskType domain.TaskType) ([]domain.Task, error) {
	query := `SELECT ` + taskColumns + ` FROM task WHERE type = $1 AND status = $2 ORDER BY created_at`
	rows, err := repo.db.Query(ctx, query, taskType, domain.TaskStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("task list running: %w", err)
	}
	defer rows.Close()

	tasks := make([]domain.Task, 0)
	for rows.Next() {
		task, scanError := scanTaskRow(rows)
		if scanError != nil {
			return nil, fmt.Errorf("task list running scan: %w", scanError)
		}
		tasks = append(tasks, task)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("task list running rows: %w", rowsError)
	}
	return tasks, nil
}

// Complete sets a task to a terminal status with an optional error message
// and a finished_at timestamp.
func (repo *TaskRepo) Complete(ctx context.Context, taskID uuid.UUID, status domain.TaskStatus, errMsg *string) error {
	const query = `UPDATE task SET status = $2, error = $3, finished_at = NOW(), updated_at = NOW() WHERE id = $1`
	if _, err := repo.db.Exec(ctx, query, taskID, status, errMsg); err != nil {
		return fmt.Errorf("task complete: %w", err)
	}
	return nil
}

// GetByID returns a single task by its UUID.
func (repo *TaskRepo) GetByID(ctx context.Context, taskID uuid.UUID) (*domain.Task, error) {
	query := `SELECT ` + taskColumns + ` FROM task WHERE id = $1`
	row := repo.db.QueryRow(ctx, query, taskID)
	task, err := scanTaskRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "task", ID: taskID.String()}
		}
		return nil, fmt.Errorf("task get: %w", err)
	}
	return &task, nil
}

func scanTaskRow(scanner interface {
	Scan(dest ...any) error
},
) (domain.Task, error) {
	var task domain.Task
	err := scanner.Scan(
		&task.ID,
		&task.Type,
		&task.Status,
		&task.ProfileID,
		&task.ListingID,
		&task.Total,
		&task.Done,
		&task.FailedCount,
		&task.Error,
		&task.CreatedAt,
		&task.StartedAt,
		&task.FinishedAt,
		&task.UpdatedAt,
	)
	return task, err
}
