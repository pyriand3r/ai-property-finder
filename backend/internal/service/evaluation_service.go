package service

import (
	"context"
	"errors"
	"fmt"
	"propertyfinder/internal/ai"
	"propertyfinder/internal/db"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/events"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EvaluationResult bundles the updated listing and its fresh grades for the
// POST /api/listings/{id}/evaluate response.
type EvaluationResult struct {
	Listing *domain.Listing
	Grades  []domain.CriterionScore
}

// EvaluationService runs the single-shot evaluate_listing operation (spec
// 01-04 Layer 2.2) — no Agent, no tool use, one LLM call producing Grades
// + a computed Score — and its bulk re-evaluation counterpart (Layer 2.3).
type EvaluationService struct {
	pool     *pgxpool.Pool
	provider ai.Provider
	broker   *events.Broker
}

// NewEvaluationService constructs an EvaluationService wired to pool,
// provider, and broker (SSE progress for ReevaluateAll).
func NewEvaluationService(pool *pgxpool.Pool, provider ai.Provider, broker *events.Broker) *EvaluationService {
	return &EvaluationService{pool: pool, provider: provider, broker: broker}
}

// GetGrades returns a listing's current grade grid (spec 01-07 Layer 5.3)
// — empty if it has never been evaluated.
func (service *EvaluationService) GetGrades(ctx context.Context, listingID uuid.UUID) ([]domain.ListingGrade, error) {
	return repository.NewGradeRepo(service.pool).ListByListing(ctx, listingID)
}

// EvaluateListing grades listingID against its profile's Criteria, replaces
// its listing_criterion_grade rows, recomputes overall_score, and clears
// needs_reevaluation — all in one transaction. Grades are mapped back to
// Criteria by position, never by an ID the model echoes (ADR 0006).
func (service *EvaluationService) EvaluateListing(ctx context.Context, listingID uuid.UUID) (*EvaluationResult, error) {
	listingRepo := repository.NewListingRepo(service.pool)
	listing, err := listingRepo.GetByID(ctx, listingID)
	if err != nil {
		return nil, err
	}

	criteria, err := repository.NewCriterionRepo(service.pool).ListByProfile(ctx, listing.ProfileID)
	if err != nil {
		return nil, err
	}
	if len(criteria) == 0 {
		return nil, &domain.ValidationError{Field: "criteria", Message: "Profil hat keine Kriterien — zuerst Kriterien anlegen"}
	}

	scores, err := service.provider.EvaluateListing(ctx, *listing, criteria)
	if err != nil {
		return nil, mapProviderError(err)
	}

	txError := db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		if replaceError := repository.NewGradeRepo(tx).ReplaceForListing(ctx, listingID, scores); replaceError != nil {
			return replaceError
		}
		if recomputeError := repository.NewListingRepo(tx).RecomputeScores(ctx, listing.ProfileID); recomputeError != nil {
			return recomputeError
		}
		return repository.NewListingRepo(tx).ClearNeedsReevaluation(ctx, listingID)
	})
	if txError != nil {
		return nil, txError
	}

	updated, err := listingRepo.GetByID(ctx, listingID)
	if err != nil {
		return nil, err
	}
	return &EvaluationResult{Listing: updated, Grades: scores}, nil
}

// ReevaluateAll finds every listing in profileID with needs_reevaluation =
// true, creates a Task row, and evaluates them one by one in a detached
// goroutine (spec 01-04 Layer 2.3: "sequenzielle Goroutine", not a polling
// worker — ADR 0008's fire-and-forget shape applies here too). Progress is
// published on the broker as task.progress / task.finished events; the
// task row is the durable source of truth for reconnect reconciliation.
func (service *EvaluationService) ReevaluateAll(ctx context.Context, profileID uuid.UUID) (*domain.Task, error) {
	stale := true
	listings, err := repository.NewListingRepo(service.pool).ListByProfile(ctx, profileID, &stale)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	total := len(listings)
	task := &domain.Task{
		ID:        uuid.New(),
		Type:      domain.TaskTypeListingsReevaluateAll,
		Status:    domain.TaskStatusRunning,
		ProfileID: &profileID,
		Total:     &total,
		CreatedAt: now,
		StartedAt: &now,
		UpdatedAt: now,
	}
	if createError := repository.NewTaskRepo(service.pool).Create(ctx, task); createError != nil {
		return nil, createError
	}
	service.broker.Publish("task.started", taskEventPayload(task))

	// Detached from the request context on purpose (ADR 0008 fire-and-
	// forget shape): the goroutine must keep running after the kickoff
	// HTTP response is sent, not get canceled when the request ends.
	//nolint:gosec,contextcheck // G118/non-inherited context: intentional (ADR 0008), not an oversight — see comment above.
	go service.runReevaluateAll(context.Background(), task.ID, listings)

	return task, nil
}

// runReevaluateAll evaluates each listing sequentially, publishing a
// task.progress event after every one, then marks the task finished.
// Individual failures don't abort the batch — they're counted, so one bad
// listing (or an unconfigured LLM) doesn't hide progress on the rest.
func (service *EvaluationService) runReevaluateAll(ctx context.Context, taskID uuid.UUID, listings []domain.Listing) {
	taskRepo := repository.NewTaskRepo(service.pool)
	done, failed := 0, 0

	for _, listing := range listings {
		if _, evalError := service.EvaluateListing(ctx, listing.ID); evalError != nil {
			failed++
		} else {
			done++
		}
		_ = taskRepo.UpdateProgress(ctx, taskID, done, failed)
		service.broker.Publish("task.progress", map[string]any{
			"task_id":      taskID,
			"done":         done,
			"failed_count": failed,
			"total":        len(listings),
		})
	}

	status := domain.TaskStatusSucceeded
	var errMsg *string
	if failed > 0 {
		status = domain.TaskStatusFailed
		msg := fmt.Sprintf("%d von %d Inseraten konnten nicht neu bewertet werden", failed, len(listings))
		errMsg = &msg
	}
	_ = taskRepo.Complete(ctx, taskID, status, errMsg)

	finished, getError := taskRepo.GetByID(ctx, taskID)
	if getError != nil {
		return
	}
	service.broker.Publish("task.finished", taskEventPayload(finished))
}

func taskEventPayload(task *domain.Task) map[string]any {
	return map[string]any{
		"task_id":      task.ID,
		"type":         task.Type,
		"status":       task.Status,
		"total":        task.Total,
		"done":         task.Done,
		"failed_count": task.FailedCount,
	}
}

// mapProviderError translates ai package sentinel errors into domain errors
// the API layer already knows how to render. ErrNotConfigured is a client-
// actionable 400 (fix the LLM settings); the rest are transient upstream (LLM)
// failures, mapped to 502 rather than an opaque 500 — the request itself
// was well-formed, retrying later may succeed.
func mapProviderError(err error) error {
	switch {
	case errors.Is(err, ai.ErrNotConfigured):
		return &domain.ValidationError{Field: "llm", Message: "LLM-Provider ist nicht konfiguriert (Provider, API-Key und Modell unter Einstellungen setzen)"}
	case errors.Is(err, ai.ErrListingTooLong):
		return &domain.UpstreamError{Message: "Inserat ist zu umfangreich für das Kontextfenster des Modells"}
	case errors.Is(err, ai.ErrTimeout):
		return &domain.UpstreamError{Message: "LLM-Anfrage hat das Zeitlimit überschritten"}
	case errors.Is(err, ai.ErrParse):
		return &domain.UpstreamError{Message: "LLM-Antwort konnte nicht verarbeitet werden"}
	default:
		return err
	}
}
