package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"propertyfinder/internal/db"
	"propertyfinder/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ChatRepository persists chat_session/chat_message (spec 01-01, Layer 4.1
// message persistence only — no Agent turn yet).
type ChatRepository interface {
	// CreateSession inserts a new chat_session row (active — ended_at
	// NULL). Used both for a profile's initial session (Layer 1.1) and to
	// start a new one (Layer 4.1).
	CreateSession(ctx context.Context, id, profileID uuid.UUID) error
	// EndActiveSession sets ended_at = NOW() on the profile's current
	// active session, if any (a fresh profile has one; this is a no-op
	// otherwise).
	EndActiveSession(ctx context.Context, profileID uuid.UUID) error
	// GetActiveSession returns the profile's current active session
	// (ended_at IS NULL).
	GetActiveSession(ctx context.Context, profileID uuid.UUID) (*domain.ChatSession, error)
	// GetSession returns a single session by ID, regardless of whether
	// it's active or historical.
	GetSession(ctx context.Context, sessionID uuid.UUID) (*domain.ChatSession, error)
	// ListSessions returns every session for a profile, most recently
	// created first.
	ListSessions(ctx context.Context, profileID uuid.UUID) ([]domain.ChatSession, error)
	// CreateMessage inserts a chat_message row.
	CreateMessage(ctx context.Context, message *domain.ChatMessage) error
	// ListMessages returns a session's messages in chronological order.
	ListMessages(ctx context.Context, sessionID uuid.UUID) ([]domain.ChatMessage, error)
	// GetMessage returns a single message by ID — used by the tool-call
	// confirm/reject endpoints (ADR 0009) to read the pending proposal out
	// of tool_calls.
	GetMessage(ctx context.Context, id uuid.UUID) (*domain.ChatMessage, error)
	// UpdateToolCalls overwrites a message's tool_calls column — used by
	// the confirm/reject endpoints to record a proposal's resolved status
	// in place (ADR 0017), so it survives a reload instead of resetting to
	// pending every time the Confirm-Card remounts.
	UpdateToolCalls(ctx context.Context, id uuid.UUID, toolCalls json.RawMessage) error
	// UpdateTokenCount sets a session's token_count_estimate from the
	// summed Provider usage of the Agent turn that just completed on it
	// (spec 00-03 Layer 6.3 — feeds the chat header's donut chart).
	UpdateTokenCount(ctx context.Context, sessionID uuid.UUID, tokenCount int) error
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

func (repo *ChatRepo) EndActiveSession(ctx context.Context, profileID uuid.UUID) error {
	const query = `
UPDATE chat_session SET ended_at = NOW(), updated_at = NOW()
WHERE profile_id = $1 AND ended_at IS NULL
`
	if _, err := repo.db.Exec(ctx, query, profileID); err != nil {
		return fmt.Errorf("chat end active session: %w", err)
	}
	return nil
}

const chatSessionColumns = `id, profile_id, created_at, updated_at, ended_at, token_count_estimate`

func (repo *ChatRepo) GetActiveSession(ctx context.Context, profileID uuid.UUID) (*domain.ChatSession, error) {
	query := `SELECT ` + chatSessionColumns + ` FROM chat_session WHERE profile_id = $1 AND ended_at IS NULL`
	row := repo.db.QueryRow(ctx, query, profileID)
	session, err := scanChatSessionRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "active chat_session", ID: profileID.String()}
		}
		return nil, fmt.Errorf("chat get active session: %w", err)
	}
	return &session, nil
}

func (repo *ChatRepo) GetSession(ctx context.Context, sessionID uuid.UUID) (*domain.ChatSession, error) {
	query := `SELECT ` + chatSessionColumns + ` FROM chat_session WHERE id = $1`
	row := repo.db.QueryRow(ctx, query, sessionID)
	session, err := scanChatSessionRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "chat_session", ID: sessionID.String()}
		}
		return nil, fmt.Errorf("chat get session: %w", err)
	}
	return &session, nil
}

func (repo *ChatRepo) ListSessions(ctx context.Context, profileID uuid.UUID) ([]domain.ChatSession, error) {
	query := `SELECT ` + chatSessionColumns + ` FROM chat_session WHERE profile_id = $1 ORDER BY created_at DESC`
	rows, err := repo.db.Query(ctx, query, profileID)
	if err != nil {
		return nil, fmt.Errorf("chat list sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]domain.ChatSession, 0)
	for rows.Next() {
		session, scanError := scanChatSessionRow(rows)
		if scanError != nil {
			return nil, fmt.Errorf("chat list sessions scan: %w", scanError)
		}
		sessions = append(sessions, session)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("chat list sessions rows: %w", rowsError)
	}
	return sessions, nil
}

const chatMessageColumns = `id, session_id, role, content, tool_calls, tool_result, tool_name, attachment_path, listing_id, created_at`

func (repo *ChatRepo) CreateMessage(ctx context.Context, message *domain.ChatMessage) error {
	query := `INSERT INTO chat_message (` + chatMessageColumns + `) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err := repo.db.Exec(ctx, query,
		message.ID,
		message.SessionID,
		message.Role,
		message.Content,
		message.ToolCalls,
		message.ToolResult,
		message.ToolName,
		message.AttachmentPath,
		message.ListingID,
		message.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("chat create message: %w", err)
	}
	return nil
}

func (repo *ChatRepo) ListMessages(ctx context.Context, sessionID uuid.UUID) ([]domain.ChatMessage, error) {
	query := `SELECT ` + chatMessageColumns + ` FROM chat_message WHERE session_id = $1 ORDER BY created_at`
	rows, err := repo.db.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("chat list messages: %w", err)
	}
	defer rows.Close()

	messages := make([]domain.ChatMessage, 0)
	for rows.Next() {
		message, scanError := scanChatMessageRow(rows)
		if scanError != nil {
			return nil, fmt.Errorf("chat list messages scan: %w", scanError)
		}
		messages = append(messages, message)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("chat list messages rows: %w", rowsError)
	}
	return messages, nil
}

func (repo *ChatRepo) GetMessage(ctx context.Context, id uuid.UUID) (*domain.ChatMessage, error) {
	query := `SELECT ` + chatMessageColumns + ` FROM chat_message WHERE id = $1`
	row := repo.db.QueryRow(ctx, query, id)
	message, err := scanChatMessageRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "chat_message", ID: id.String()}
		}
		return nil, fmt.Errorf("chat get message: %w", err)
	}
	return &message, nil
}

func (repo *ChatRepo) UpdateToolCalls(ctx context.Context, id uuid.UUID, toolCalls json.RawMessage) error {
	const query = `UPDATE chat_message SET tool_calls = $2 WHERE id = $1`
	result, err := repo.db.Exec(ctx, query, id, toolCalls)
	if err != nil {
		return fmt.Errorf("chat update tool calls: %w", err)
	}
	if result.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "chat_message", ID: id.String()}
	}
	return nil
}

func (repo *ChatRepo) UpdateTokenCount(ctx context.Context, sessionID uuid.UUID, tokenCount int) error {
	const query = `UPDATE chat_session SET token_count_estimate = $2, updated_at = NOW() WHERE id = $1`
	if _, err := repo.db.Exec(ctx, query, sessionID, tokenCount); err != nil {
		return fmt.Errorf("chat update token count: %w", err)
	}
	return nil
}

func scanChatSessionRow(scanner interface {
	Scan(dest ...any) error
},
) (domain.ChatSession, error) {
	var session domain.ChatSession
	err := scanner.Scan(
		&session.ID,
		&session.ProfileID,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.EndedAt,
		&session.TokenCountEstimate,
	)
	return session, err
}

func scanChatMessageRow(scanner interface {
	Scan(dest ...any) error
},
) (domain.ChatMessage, error) {
	var message domain.ChatMessage
	err := scanner.Scan(
		&message.ID,
		&message.SessionID,
		&message.Role,
		&message.Content,
		&message.ToolCalls,
		&message.ToolResult,
		&message.ToolName,
		&message.AttachmentPath,
		&message.ListingID,
		&message.CreatedAt,
	)
	return message, err
}
