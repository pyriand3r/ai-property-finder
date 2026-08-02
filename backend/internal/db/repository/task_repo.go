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
// create a row, update its counters, and mark it finished.
type TaskRepository interface {
	Create(ctx context.Context, task *domain.Task) error
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
	query := `INSERT INTO task (` + taskColumns + `, payload) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, '{}'::jsonb)`
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
