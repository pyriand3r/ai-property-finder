package service

import (
	"context"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ContextService orchestrates Context Files and their revision history
// (spec 01-02 Layer 1.2 — direct user edits only; Agent-proposed changes
// land with the chat layer).
type ContextService struct {
	pool *pgxpool.Pool
}

// NewContextService constructs a ContextService wired to pool.
func NewContextService(pool *pgxpool.Pool) *ContextService {
	return &ContextService{pool: pool}
}

// GetGlobal returns the single Global Context.
func (service *ContextService) GetGlobal(ctx context.Context) (*domain.Context, error) {
	return repository.NewContextRepo(service.pool).GetGlobal(ctx)
}

// GetProfileContext returns the Profile Context owned by profileID.
func (service *ContextService) GetProfileContext(ctx context.Context, profileID uuid.UUID) (*domain.Context, error) {
	return repository.NewContextRepo(service.pool).GetByProfileID(ctx, profileID)
}

// UpdateGlobal overwrites the Global Context's content_md. A
// context_revision row is written only when the content actually changed
// (spec 00-02), snapshotting the text as it was before this write.
func (service *ContextService) UpdateGlobal(ctx context.Context, contentMD string) (*domain.Context, error) {
	repo := repository.NewContextRepo(service.pool)
	current, err := repo.GetGlobal(ctx)
	if err != nil {
		return nil, err
	}
	return service.updateContent(ctx, repo, current, contentMD, domain.ChangedByUser, nil)
}

// UpdateProfileContext overwrites a Profile Context's content_md, same
// revision semantics as UpdateGlobal.
func (service *ContextService) UpdateProfileContext(ctx context.Context, profileID uuid.UUID, contentMD string) (*domain.Context, error) {
	repo := repository.NewContextRepo(service.pool)
	current, err := repo.GetByProfileID(ctx, profileID)
	if err != nil {
		return nil, err
	}
	return service.updateContent(ctx, repo, current, contentMD, domain.ChangedByUser, nil)
}

// UpdateGlobalFromAgent and UpdateProfileContextFromAgent apply an
// Agent-proposed update_context change once the user has confirmed it
// (ADR 0009/0004, Layer 6.5) — same write path as the manual editor above,
// but the resulting context_revision is attributed changed_by='agent' and
// linked to the confirming chat_message for the history panel.
func (service *ContextService) UpdateGlobalFromAgent(ctx context.Context, contentMD string, chatMessageID uuid.UUID) (*domain.Context, error) {
	repo := repository.NewContextRepo(service.pool)
	current, err := repo.GetGlobal(ctx)
	if err != nil {
		return nil, err
	}
	return service.updateContent(ctx, repo, current, contentMD, domain.ChangedByAgent, &chatMessageID)
}

func (service *ContextService) UpdateProfileContextFromAgent(ctx context.Context, profileID uuid.UUID, contentMD string, chatMessageID uuid.UUID) (*domain.Context, error) {
	repo := repository.NewContextRepo(service.pool)
	current, err := repo.GetByProfileID(ctx, profileID)
	if err != nil {
		return nil, err
	}
	return service.updateContent(ctx, repo, current, contentMD, domain.ChangedByAgent, &chatMessageID)
}

// ListProfileHistory returns a Profile Context's revisions, newest first.
func (service *ContextService) ListProfileHistory(ctx context.Context, profileID uuid.UUID) ([]domain.ContextRevision, error) {
	repo := repository.NewContextRepo(service.pool)
	current, err := repo.GetByProfileID(ctx, profileID)
	if err != nil {
		return nil, err
	}
	return repo.ListRevisions(ctx, current.ID)
}

func (service *ContextService) updateContent(
	ctx context.Context,
	repo *repository.ContextRepo,
	current *domain.Context,
	newContentMD string,
	changedBy domain.ChangedBy,
	chatMessageID *uuid.UUID,
) (*domain.Context, error) {
	if newContentMD != current.ContentMD {
		revision := &domain.ContextRevision{
			ID:            uuid.New(),
			ContextID:     current.ID,
			ContentMD:     current.ContentMD,
			ChangedBy:     changedBy,
			ChatMessageID: chatMessageID,
			CreatedAt:     time.Now().UTC(),
		}
		if revisionError := repo.CreateRevision(ctx, revision); revisionError != nil {
			return nil, revisionError
		}
	}
	if updateError := repo.UpdateContent(ctx, current.ID, newContentMD); updateError != nil {
		return nil, updateError
	}
	current.ContentMD = newContentMD
	current.UpdatedAt = time.Now().UTC()
	return current, nil
}
