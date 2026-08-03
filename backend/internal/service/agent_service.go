package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/config"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/events"
	"propertyfinder/internal/fetch"
	"propertyfinder/internal/geocode"
	"propertyfinder/internal/pdf"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentService drives one Agent turn per user message: assembles the
// system prompt, replays session history through the Provider, dispatches
// Tool calls (Layer 6.4/6.5: fetch_url/save_listing/download_image
// /evaluate_listing/propose_criteria/update_context), streams progress
// over the SSE broker, and persists every roundtrip's assistant/tool
// chat_message rows (spec 00-02, ADR 0007/0008/0010) — not just the
// turn's final reply, so a later turn in the same session replays a
// complete history and pending-confirmation tool calls (Layer 6.5,
// ADR 0009) are readable from the DB.
type AgentService struct {
	pool       *pgxpool.Pool
	chat       *ChatService
	evaluation *EvaluationService
	provider   agent.Provider
	broker     *events.Broker
	llm        config.LLM
	dataDir    string
}

// NewAgentService constructs an AgentService wired to pool, provider (the
// resolved agent.Provider — an unconfigured stand-in when llm.api_key is
// empty, per agent.NewUnconfiguredProvider), broker, the current
// llm.max_llm_roundtrips / llm.max_tool_calls_per_turn config, and dataDir
// (ADR 0012 — where download_image writes files, same volume root as
// ImageService). evaluation backs the evaluate_listing Tool — the caller
// passes the same *EvaluationService instance wired to the
// POST /api/listings/{id}/evaluate endpoint (Layer 2.2), not a second one,
// so there's exactly one place that single-shot Grade logic lives.
func NewAgentService(pool *pgxpool.Pool, provider agent.Provider, broker *events.Broker, llm config.LLM, dataDir string, evaluation *EvaluationService) *AgentService {
	return &AgentService{pool: pool, chat: NewChatService(pool), evaluation: evaluation, provider: provider, broker: broker, llm: llm, dataDir: dataDir}
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

	tools := agent.Tools()
	registry := &agent.Registry{
		ProfileID:  session.ProfileID,
		Listings:   repository.NewListingRepo(service.pool),
		Images:     repository.NewImageRepo(service.pool),
		Fetcher:    fetch.NewClient(),
		Downloader: fetch.NewClient(),
		DataDir:    service.dataDir,
		Evaluator:  evaluationAdapter{service.evaluation},
		Geocoder:   geocodeAdapter{},
	}
	loop := &agent.Loop{
		Provider:            service.provider,
		MaxLLMRoundtrips:    service.llm.MaxLLMRoundtrips,
		MaxToolCallsPerTurn: service.llm.MaxToolCallsPerTurn,
		Dispatch:            registry.Dispatch,
	}
	req := agent.TurnRequest{
		System:   agent.BuildSystemPrompt(len(tools) > 0, agent.EffectiveMaxLLMRoundtrips(service.llm.MaxLLMRoundtrips), globalContext.ContentMD, profileContext.ContentMD),
		Messages: service.provider.FormatMessages(history),
		Tools:    tools,
	}

	// A PDF attached to the triggering user message is read once here and
	// appended as a native document content block to that message — not
	// requested via a Tool call (spec 00-03 "PDF Handling"; read_pdf isn't
	// in the Tools() catalog for exactly this reason). Deleted from disk
	// once the turn ends either way, so it's never resent on a later turn.
	pdfAttachment := attachedPDFPath(history)
	if pdfAttachment != nil && service.provider.SupportsDocumentInput() {
		if encoded, readErr := pdf.ReadBase64(service.dataDir, *pdfAttachment); readErr != nil {
			log.Printf("agent: read pdf attachment for session %s: %v", sessionID, readErr)
		} else if last := len(req.Messages) - 1; last >= 0 {
			req.Messages[last].Content = append(req.Messages[last].Content, agent.ContentBlock{
				Type:         agent.ContentBlockDocument,
				DocumentData: encoded,
			})
		}
	}

	onEvent := func(event agent.ChatEvent) {
		switch {
		case event.Type == agent.ChatEventTextDelta:
			service.broker.Publish("chat.text_delta", chatTextDeltaPayload{SessionID: sessionID, Delta: event.Text})
		case event.Type == agent.ChatEventToolCallRequest && event.ToolCall != nil:
			// Tool dispatch (a real HTTP fetch/DB write) can take several
			// seconds; without this the chat goes silent between the last
			// text_delta and the next roundtrip's reply (spec 00-03: "each
			// intermediate tool call is also streamed as a progress event").
			service.broker.Publish("chat.tool_progress", chatToolProgressPayload{SessionID: sessionID, Tool: event.ToolCall.Name})
		}
	}

	onRoundtrip := func(outcome agent.RoundtripOutcome) {
		service.persistRoundtrip(ctx, chatRepo, sessionID, registry, outcome)
	}

	result, err := loop.Run(ctx, req, onEvent, onRoundtrip)

	if pdfAttachment != nil {
		if deleteErr := pdf.Delete(service.dataDir, *pdfAttachment); deleteErr != nil {
			log.Printf("agent: delete pdf attachment for session %s: %v", sessionID, deleteErr)
		}
	}

	if err != nil {
		service.publishError(sessionID, err)
		return
	}

	tokenCount := result.Usage.InputTokens + result.Usage.OutputTokens
	if updateErr := chatRepo.UpdateTokenCount(ctx, sessionID, tokenCount); updateErr != nil {
		log.Printf("agent: update token count for session %s: %v", sessionID, updateErr)
	}
}

// persistRoundtrip writes one roundtrip's assistant message (text and/or
// tool_calls) and, once dispatched, one role=tool row per tool outcome
// (spec 00-02, ADR 0010) — every roundtrip, not just the turn's last one,
// so the next turn's history replay is complete. The turn-ending roundtrip
// (Results empty) is the one that streams as "chat.message" to the
// frontend and carries the inline Listing card; intermediate
// tool-calling roundtrips still get their own DB rows (for replay and for
// Layer 6.5's confirm-cards to read propose_criteria/update_context
// arguments from) but aren't separately pushed as a visible chat bubble
// beyond the tool_progress event already sent for each call.
func (service *AgentService) persistRoundtrip(ctx context.Context, chatRepo *repository.ChatRepo, sessionID uuid.UUID, registry *agent.Registry, outcome agent.RoundtripOutcome) {
	isFinal := len(outcome.Results) == 0

	assistantMessage := &domain.ChatMessage{
		ID:        uuid.New(),
		SessionID: sessionID,
		Role:      domain.ChatRoleAssistant,
		Content:   textFromBlocks(outcome.Assistant.Content),
		ToolCalls: toolCallsJSON(outcome.Assistant.Content, outcome.Results),
		CreatedAt: time.Now().UTC(),
	}
	if isFinal {
		assistantMessage.ListingID = registry.TouchedListingID()
	}
	if createErr := chatRepo.CreateMessage(ctx, assistantMessage); createErr != nil {
		log.Printf("agent: persist assistant message for session %s: %v", sessionID, createErr)
		return
	}
	if isFinal {
		service.broker.Publish("chat.message", chatMessagePayload{SessionID: sessionID, Message: assistantMessage})
	}

	for _, toolOutcome := range outcome.Results {
		toolName := toolOutcome.Call.Name
		toolMessage := &domain.ChatMessage{
			ID:         uuid.New(),
			SessionID:  sessionID,
			Role:       domain.ChatRoleTool,
			Content:    humanizeToolResult(toolName, toolOutcome.Result, toolOutcome.IsError),
			ToolResult: toolResultJSON(toolOutcome.Call.ID, toolOutcome.Result),
			ToolName:   &toolName,
			CreatedAt:  time.Now().UTC(),
		}
		if createErr := chatRepo.CreateMessage(ctx, toolMessage); createErr != nil {
			log.Printf("agent: persist tool result for session %s: %v", sessionID, createErr)
		}
	}
}

// textFromBlocks concatenates every text block in an assistant Message —
// there is at most one per roundtrip (see assistantResponseBlocks), but
// this stays correct if that ever changes.
func textFromBlocks(blocks []agent.ContentBlock) string {
	var text strings.Builder
	for _, block := range blocks {
		if block.Type == agent.ContentBlockText {
			text.WriteString(block.Text)
		}
	}
	return text.String()
}

// toolCallsJSON extracts the tool_use blocks from an assistant Message
// into the StoredToolCall shape chat_message.tool_calls persists (nil —
// not an empty array — when the roundtrip made no tool calls, matching
// the column's NULL-when-absent convention). results is a defensive
// cross-check: every ToolOutcome that was actually dispatched is appended
// if its call ID didn't already come through in blocks. Without this, an
// occasional streaming/roundtrip-persistence gap can produce an assistant
// row with fewer tool_calls than were really dispatched — the
// corresponding role=tool row (built independently from results, not
// blocks) still gets its own chat_message with a real ToolResultForID,
// and a later turn's history replay then contains a tool_result with no
// matching tool_use in the previous message, which Claude's API rejects
// outright ("Each tool_result block must have a corresponding tool_use
// block in the previous message"). Observed intermittently in Layer 6.6
// live testing (evaluate_listing x2 then compute_ranking in one turn);
// this makes the persisted record correct regardless of that gap's exact
// cause.
func toolCallsJSON(blocks []agent.ContentBlock, results []agent.ToolOutcome) json.RawMessage {
	var calls []agent.StoredToolCall
	seen := make(map[string]bool, len(blocks))
	for _, block := range blocks {
		if block.Type == agent.ContentBlockToolUse {
			calls = append(calls, agent.StoredToolCall{ID: block.ToolUseID, Name: block.ToolName, Input: block.ToolInput})
			seen[block.ToolUseID] = true
		}
	}
	for _, outcome := range results {
		if !seen[outcome.Call.ID] {
			calls = append(calls, agent.StoredToolCall{ID: outcome.Call.ID, Name: outcome.Call.Name, Input: outcome.Call.Arguments})
		}
	}
	if len(calls) == 0 {
		return nil
	}
	data, err := json.Marshal(calls)
	if err != nil {
		return nil
	}
	return data
}

// toolResultJSON builds chat_message.tool_result's
// {"tool_call_id":"...","result":...} shape (ADR 0010). result is already
// valid JSON — every Registry tool implementation returns it via
// json.Marshal — so it's spliced in raw rather than re-encoded as a
// string.
func toolResultJSON(callID, result string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"tool_call_id":%q,"result":%s}`, callID, result))
}

// humanizeToolResult renders a short, chat-readable line for a tool
// -result bubble instead of the raw JSON a Tool returns. Falls back to
// just the tool name if the result has none of the fields it knows to
// look for — every Registry tool's JSON shape includes at least one.
func humanizeToolResult(toolName, resultJSON string, isError bool) string {
	var parsed struct {
		Status  string `json:"status"`
		Summary string `json:"summary"`
		Note    string `json:"note"`
		Error   string `json:"error"`
	}
	_ = json.Unmarshal([]byte(resultJSON), &parsed)
	switch {
	case isError && parsed.Error != "":
		return fmt.Sprintf("%s: %s", toolName, parsed.Error)
	case parsed.Summary != "":
		return parsed.Summary
	case parsed.Note != "":
		return fmt.Sprintf("%s — %s", toolName, parsed.Note)
	case parsed.Status != "":
		return fmt.Sprintf("%s: %s", toolName, parsed.Status)
	default:
		return toolName
	}
}

// attachedPDFPath returns the triggering user message's attachment_path if
// it names a PDF, or nil. history is chronological, so that message is
// always the last row — SendMessage persists the user's message before
// runTurn is ever spawned (ADR 0008), and this function only runs once per
// turn on the history as of that point.
func attachedPDFPath(history []domain.ChatMessage) *string {
	if len(history) == 0 {
		return nil
	}
	last := history[len(history)-1]
	if last.Role != domain.ChatRoleUser || last.AttachmentPath == nil {
		return nil
	}
	if !strings.HasSuffix(strings.ToLower(*last.AttachmentPath), ".pdf") {
		return nil
	}
	return last.AttachmentPath
}

// geocodeAdapter satisfies agent.AddressGeocoder against the
// package-level geocode.Geocode func.
type geocodeAdapter struct{}

func (geocodeAdapter) Geocode(ctx context.Context, address, country string) (*geocode.Result, error) {
	return geocode.Geocode(ctx, address, country)
}

// evaluationAdapter satisfies agent.ListingEvaluator against
// *EvaluationService without internal/agent importing internal/service
// (which would cycle — service already imports agent for AgentService).
type evaluationAdapter struct {
	service *EvaluationService
}

func (a evaluationAdapter) EvaluateListing(ctx context.Context, listingID uuid.UUID) (*domain.Listing, []domain.CriterionScore, error) {
	result, err := a.service.EvaluateListing(ctx, listingID)
	if err != nil {
		return nil, nil, err
	}
	return result.Listing, result.Grades, nil
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

type chatToolProgressPayload struct {
	SessionID uuid.UUID `json:"session_id"`
	Tool      string    `json:"tool"`
}

type chatMessagePayload struct {
	SessionID uuid.UUID           `json:"session_id"`
	Message   *domain.ChatMessage `json:"message"`
}

type chatTurnErrorPayload struct {
	SessionID uuid.UUID `json:"session_id"`
	Message   string    `json:"message"`
}
