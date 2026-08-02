package repository

import (
	"context"
	"fmt"
	"propertyfinder/internal/db"

	"github.com/google/uuid"
)

// ChatRepository persists chat_session/chat_message. Layer 1 only needs
// CreateSession (a profile owns exactly one active session from creation
// onward); message persistence lands in Layer 4.
type ChatRepository interface {
	// CreateSession inserts the initial chat_session row for a newly
	// created Search Profile.
	CreateSession(ctx context.Context, id, profileID uuid.UUID) error
}

// ChatRepo is the pgx-backed implementation of ChatRepository.
type ChatRepo struct {
	db db.Querier
}

// NewChatRepo constructs a ChatRepo wired to q — either the pool or a
// transaction.
func NewChatRepo(q db.Querier) *ChatRepo {
	return &ChatRepo{db: q}
}

func (repo *ChatRepo) CreateSession(ctx context.Context, id, profileID uuid.UUID) error {
	const query = `
INSERT INTO chat_session (id, profile_id, created_at, updated_at)
VALUES ($1, $2, NOW(), NOW())
`
	if _, err := repo.db.Exec(ctx, query, id, profileID); err != nil {
		return fmt.Errorf("chat create session: %w", err)
	}
	return nil
}
