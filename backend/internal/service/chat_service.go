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

// ChatService persists Chat Sessions and user messages (spec 01-01 Layer
// 4.1 — no Agent turn yet, so this only ever writes role=user rows).
type ChatService struct {
	pool *pgxpool.Pool
}

// NewChatService constructs a ChatService wired to pool.
func NewChatService(pool *pgxpool.Pool) *ChatService {
	return &ChatService{pool: pool}
}

// ListSessions returns every session for a profile, most recently created
// first.
func (service *ChatService) ListSessions(ctx context.Context, profileID uuid.UUID) ([]domain.ChatSession, error) {
	if _, err := repository.NewProfileRepo(service.pool).GetByID(ctx, profileID); err != nil {
		return nil, err
	}
	return repository.NewChatRepo(service.pool).ListSessions(ctx, profileID)
}

// StartNewSession ends the profile's current active session (if any) and
// starts a fresh one, atomically — the old session becomes read-only
// history (spec 01-01).
func (service *ChatService) StartNewSession(ctx context.Context, profileID uuid.UUID) (*domain.ChatSession, error) {
	if _, err := repository.NewProfileRepo(service.pool).GetByID(ctx, profileID); err != nil {
		return nil, err
	}

	newID := uuid.New()
	txError := db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		chatRepo := repository.NewChatRepo(tx)
		if endError := chatRepo.EndActiveSession(ctx, profileID); endError != nil {
			return endError
		}
		return chatRepo.CreateSession(ctx, newID, profileID)
	})
	if txError != nil {
		return nil, txError
	}
	return repository.NewChatRepo(service.pool).GetSession(ctx, newID)
}

// ListMessages returns a session's messages in chronological order,
// verifying the session actually belongs to profileID first.
func (service *ChatService) ListMessages(ctx context.Context, profileID, sessionID uuid.UUID) ([]domain.ChatMessage, error) {
	session, err := repository.NewChatRepo(service.pool).GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.ProfileID != profileID {
		return nil, &domain.NotFoundError{Entity: "chat_session", ID: sessionID.String()}
	}
	return repository.NewChatRepo(service.pool).ListMessages(ctx, sessionID)
}

// SendMessage persists a user message on sessionID. The session must
// belong to profileID and still be active — posting into an ended
// (read-only history) session is rejected.
func (service *ChatService) SendMessage(ctx context.Context, profileID, sessionID uuid.UUID, content string, attachmentPath *string) (*domain.ChatMessage, error) {
	content = strings.TrimSpace(content)
	if content == "" && attachmentPath == nil {
		return nil, &domain.ValidationError{Field: "content", Message: "Nachricht oder Anhang ist erforderlich"}
	}

	chatRepo := repository.NewChatRepo(service.pool)
	session, err := chatRepo.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.ProfileID != profileID {
		return nil, &domain.NotFoundError{Entity: "chat_session", ID: sessionID.String()}
	}
	if session.EndedAt != nil {
		return nil, &domain.ConflictError{Message: "Session ist beendet — nur die aktive Session akzeptiert neue Nachrichten"}
	}

	message := &domain.ChatMessage{
		ID:             uuid.New(),
		SessionID:      sessionID,
		Role:           domain.ChatRoleUser,
		Content:        content,
		AttachmentPath: attachmentPath,
		CreatedAt:      time.Now().UTC(),
	}
	if createError := chatRepo.CreateMessage(ctx, message); createError != nil {
		return nil, createError
	}
	return message, nil
}
