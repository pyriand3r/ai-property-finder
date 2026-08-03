package service

import (
	"context"
	"log"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/config"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/events"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentService drives one Agent turn per user message: assembles the
// system prompt, replays session history through the Provider, streams
// progress over the SSE broker, and persists the final assistant reply
// (spec 00-03, ADR 0007/0008). Layer 6.3 ships with no Tool catalog —
// every turn is exactly one LLM roundtrip, replying with text only.
type AgentService struct {
	pool     *pgxpool.Pool
	chat     *ChatService
	provider agent.Provider
	broker   *events.Broker
	llm      config.LLM
}

// NewAgentService constructs an AgentService wired to pool, provider (the
// resolved agent.Provider — an unconfigured stand-in when llm.api_key is
// empty, per agent.NewUnconfiguredProvider), broker, and the current
// llm.max_llm_roundtrips / llm.max_tool_calls_per_turn config.
func NewAgentService(pool *pgxpool.Pool, provider agent.Provider, broker *events.Broker, llm config.LLM) *AgentService {
	return &AgentService{pool: pool, chat: NewChatService(pool), provider: provider, broker: broker, llm: llm}
}

// SendMessage persists the user's message synchronously — so a message is
// never lost even if the server dies immediately after (ADR 0008) — then
// spawns the Agent turn in a goroutine detached from ctx via its own
// context.Background(): a closed browser tab or an HTTP request timeout
// must not cut the turn short once the user's message is committed. The
// caller (the HTTP handler) returns as soon as this call returns; the
// reply streams over the SSE broker and lands as a chat_message once the
// turn ends.
func (service *AgentService) SendMessage(ctx context.Context, profileID, sessionID uuid.UUID, content string, attachmentPath *string) (*domain.ChatMessage, error) {
	message, err := service.chat.SendMessage(ctx, profileID, sessionID, content, attachmentPath)
	if err != nil {
		return nil, err
	}
	//nolint:gosec,contextcheck // G118/non-inherited context: intentional (ADR 0008), not an oversight — see comment above.
	go service.runTurn(context.Background(), sessionID)
	return message, nil
}

func (service *AgentService) runTurn(ctx context.Context, sessionID uuid.UUID) {
	chatRepo := repository.NewChatRepo(service.pool)
	session, err := chatRepo.GetSession(ctx, sessionID)
	if err != nil {
		service.publishError(sessionID, err)
		return
	}

	contextRepo := repository.NewContextRepo(service.pool)
	globalContext, err := contextRepo.GetGlobal(ctx)
	if err != nil {
		service.publishError(sessionID, err)
		return
	}
	profileContext, err := contextRepo.GetByProfileID(ctx, session.ProfileID)
	if err != nil {
		service.publishError(sessionID, err)
		return
	}

	history, err := chatRepo.ListMessages(ctx, sessionID)
	if err != nil {
		service.publishError(sessionID, err)
		return
	}

	const hasTools = false // Layer 6.4 flips this once the catalog exists.
	loop := &agent.Loop{
		Provider:            service.provider,
		MaxLLMRoundtrips:    service.llm.MaxLLMRoundtrips,
		MaxToolCallsPerTurn: service.llm.MaxToolCallsPerTurn,
	}
	req := agent.TurnRequest{
		System:   agent.BuildSystemPrompt(hasTools, agent.EffectiveMaxLLMRoundtrips(service.llm.MaxLLMRoundtrips), globalContext.ContentMD, profileContext.ContentMD),
		Messages: service.provider.FormatMessages(history),
	}

	result, err := loop.Run(ctx, req, func(event agent.ChatEvent) {
		if event.Type == agent.ChatEventTextDelta {
			service.broker.Publish("chat.text_delta", chatTextDeltaPayload{SessionID: sessionID, Delta: event.Text})
		}
	})
	if err != nil {
		service.publishError(sessionID, err)
		return
	}

	assistantMessage := &domain.ChatMessage{
		ID:        uuid.New(),
		SessionID: sessionID,
		Role:      domain.ChatRoleAssistant,
		Content:   result.Text,
		CreatedAt: time.Now().UTC(),
	}
	if createErr := chatRepo.CreateMessage(ctx, assistantMessage); createErr != nil {
		service.publishError(sessionID, createErr)
		return
	}

	tokenCount := result.Usage.InputTokens + result.Usage.OutputTokens
	if updateErr := chatRepo.UpdateTokenCount(ctx, sessionID, tokenCount); updateErr != nil {
		log.Printf("agent: update token count for session %s: %v", sessionID, updateErr)
	}

	service.broker.Publish("chat.message", chatMessagePayload{SessionID: sessionID, Message: assistantMessage})
}

func (service *AgentService) publishError(sessionID uuid.UUID, err error) {
	log.Printf("agent: turn failed for session %s: %v", sessionID, err)
	service.broker.Publish("chat.turn_error", chatTurnErrorPayload{
		SessionID: sessionID,
		Message:   "Der Agent konnte nicht antworten. Bitte erneut versuchen.",
	})
}

type chatTextDeltaPayload struct {
	SessionID uuid.UUID `json:"session_id"`
	Delta     string    `json:"delta"`
}

type chatMessagePayload struct {
	SessionID uuid.UUID           `json:"session_id"`
	Message   *domain.ChatMessage `json:"message"`
}

type chatTurnErrorPayload struct {
	SessionID uuid.UUID `json:"session_id"`
	Message   string    `json:"message"`
}
