package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/attachment"
	"propertyfinder/internal/db"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/events"
	"propertyfinder/internal/llm"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	llm        *llm.Runtime
	broker     *events.Broker
	dataDir    string

	turnsMu sync.Mutex
	turns   map[uuid.UUID]context.CancelFunc
}

// NewAgentService constructs an AgentService wired to pool, llmRuntime (the
// live LLM configuration: each turn takes one snapshot of the agent.Provider
// — an unconfigured stand-in when llm.api_key is empty, per
// agent.NewUnconfiguredProvider — and the llm.max_llm_roundtrips /
// llm.max_tool_calls_per_turn budget, so a settings change applies from the
// next turn on and never mid-turn), broker, and dataDir
// (ADR 0012 — where download_image writes files, same volume root as
// ImageService). evaluation backs the evaluate_listing Tool — the caller
// passes the same *EvaluationService instance wired to the
// POST /api/listings/{id}/evaluate endpoint (Layer 2.2), not a second one,
// so there's exactly one place that single-shot Grade logic lives.
func NewAgentService(pool *pgxpool.Pool, llmRuntime *llm.Runtime, broker *events.Broker, dataDir string, evaluation *EvaluationService) *AgentService {
	return &AgentService{
		pool: pool, chat: NewChatService(pool), evaluation: evaluation, llm: llmRuntime, broker: broker, dataDir: dataDir,
		turns: make(map[uuid.UUID]context.CancelFunc),
	}
}

// SendMessage persists the user's message synchronously — so a message is
// never lost even if the server dies immediately after (ADR 0008) — then
// spawns the Agent turn in a goroutine detached from ctx via its own
// context.Background(): a closed browser tab or an HTTP request timeout
// must not cut the turn short once the user's message is committed. The
// caller (the HTTP handler) returns as soon as this call returns; the
// reply streams over the SSE broker and lands as a chat_message once the
// turn ends.
//
//nolint:contextcheck // non-inherited context (ADR 0008): the goroutine's turnCtx is deliberately derived from context.Background(), not ctx — see comment above.
func (service *AgentService) SendMessage(ctx context.Context, profileID, sessionID uuid.UUID, content string, attachmentPath *string) (*domain.ChatMessage, error) {
	message, err := service.chat.SendMessage(ctx, profileID, sessionID, content, attachmentPath)
	if err != nil {
		return nil, err
	}
	// turnCtx is cancelable (unlike the plain context.Background() the rest
	// of ADR 0008 still applies) so StopTurn can implement spec 01-01's
	// "stop" control without tearing down the fire-and-forget shape: a
	// closed browser tab still doesn't cut the turn short, but an explicit
	// Stop click does. Registered synchronously, before this method
	// returns — a Stop click racing the HTTP response for this very
	// SendMessage call must always find the turn already registered.
	turnCtx, cancel := context.WithCancel(context.Background())
	service.registerTurn(sessionID, cancel)
	go func() {
		defer service.unregisterTurn(sessionID)
		defer cancel()
		service.runTurn(turnCtx, sessionID)
	}()
	return message, nil
}

func (service *AgentService) registerTurn(sessionID uuid.UUID, cancel context.CancelFunc) {
	service.turnsMu.Lock()
	defer service.turnsMu.Unlock()
	service.turns[sessionID] = cancel
}

func (service *AgentService) unregisterTurn(sessionID uuid.UUID) {
	service.turnsMu.Lock()
	defer service.turnsMu.Unlock()
	delete(service.turns, sessionID)
}

// StopTurn cancels sessionID's in-flight Agent turn (spec 01-01 "stop"
// control): Loop.Run notices ctx.Err() at the next roundtrip boundary and
// wraps the turn up as a normal (not error) turn end — any tool call
// currently executing still runs to completion (agent.Loop.dispatchAll
// deliberately detaches from this cancellation). Validates sessionID
// belongs to profileID first, same ownership check ChatService.SendMessage
// applies, so one profile can't cancel another's turn by guessing a session
// ID. Returns (false, nil) if no turn is currently running for sessionID —
// expected and harmless if the turn finished naturally in the race with the
// user's click; the handler reports that as 404 rather than an error.
func (service *AgentService) StopTurn(ctx context.Context, profileID, sessionID uuid.UUID) (bool, error) {
	session, err := repository.NewChatRepo(service.pool).GetSession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if session.ProfileID != profileID {
		return false, &domain.NotFoundError{Entity: "chat_session", ID: sessionID.String()}
	}

	service.turnsMu.Lock()
	cancel, ok := service.turns[sessionID]
	service.turnsMu.Unlock()
	if !ok {
		return false, nil
	}
	cancel()
	return true, nil
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

	listingContextMD := ""
	if listingID := currentListingID(history); listingID != nil {
		if listing, listingErr := repository.NewListingRepo(service.pool).GetByID(ctx, *listingID); listingErr == nil {
			listingContextMD = agent.BuildListingContextBlock(*listing)
		}
		// A lookup failure here (deleted listing, etc.) isn't fatal to the
		// turn — the block is a proactive nicety, save_listing's own
		// skip/force_overwrite logic (ADR 0016) still protects the fields
		// even without it.
	}

	// One snapshot for the whole turn: provider, message formatting, and
	// budget must come from the same configuration generation.
	llmSnapshot := service.llm.Snapshot()
	provider := llmSnapshot.Agent
	llmConfig := llmSnapshot.Config

	tools := agent.Tools()
	registry := NewToolRegistry(service.pool, service.dataDir, service.evaluation, session.ProfileID)
	loop := &agent.Loop{
		Provider:            provider,
		MaxLLMRoundtrips:    llmConfig.MaxLLMRoundtrips,
		MaxToolCallsPerTurn: llmConfig.MaxToolCallsPerTurn,
		Dispatch:            registry.Dispatch,
	}
	req := agent.TurnRequest{
		System:   agent.BuildSystemPrompt(len(tools) > 0, agent.EffectiveMaxLLMRoundtrips(llmConfig.MaxLLMRoundtrips), globalContext.ContentMD, profileContext.ContentMD, listingContextMD),
		Messages: agent.RepairToolPairing(provider.FormatMessages(history)),
		Tools:    tools,
	}

	// A PDF attached to the triggering user message is read once here and
	// appended as a native document content block to that message — not
	// requested via a Tool call (spec 00-03 "PDF Handling"; read_pdf isn't
	// in the Tools() catalog for exactly this reason). Deleted from disk
	// once the turn ends either way, so it's never resent on a later turn.
	pdfAttachment := attachedPDFPath(history)
	if pdfAttachment != nil && provider.SupportsDocumentInput() {
		if encoded, readErr := attachment.ReadBase64(service.dataDir, *pdfAttachment); readErr != nil {
			log.Printf("agent: read pdf attachment for session %s: %v", sessionID, readErr)
		} else if last := len(req.Messages) - 1; last >= 0 {
			req.Messages[last].Content = append(req.Messages[last].Content, agent.ContentBlock{
				Type:         agent.ContentBlockDocument,
				DocumentData: encoded,
			})
		}
	}

	// An image attached the same way as a PDF above (ADR 0020) — a JPG/PNG
	// screenshot for when the user has no PDF-export option on mobile.
	imageAttachment, imageMediaType := attachedImagePath(history)
	if imageAttachment != nil && provider.SupportsImageInput() {
		if encoded, readErr := attachment.ReadBase64(service.dataDir, *imageAttachment); readErr != nil {
			log.Printf("agent: read image attachment for session %s: %v", sessionID, readErr)
		} else if last := len(req.Messages) - 1; last >= 0 {
			req.Messages[last].Content = append(req.Messages[last].Content, agent.ContentBlock{
				Type:           agent.ContentBlockImage,
				ImageData:      encoded,
				ImageMediaType: imageMediaType,
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

	// persistCtx is deliberately detached from ctx's cancellation (spec
	// 01-01 "stop" control): onRoundtrip's DB write must still succeed for
	// the roundtrip that observes the cancellation and wraps the turn up
	// (agent.Loop.stopped) — using ctx itself there would hand pgx an
	// already-canceled context and the INSERT would fail outright with
	// "context canceled", silently losing the stop note and, worse, never
	// firing chat.message, which would leave the frontend's turnInProgress
	// flag stuck forever. ADR 0008 is explicit that already-produced
	// results are kept, not rolled back, on a stop — that has to include
	// the wrap-up message a stop itself produces.
	persistCtx := context.WithoutCancel(ctx)
	onRoundtrip := func(outcome agent.RoundtripOutcome) {
		service.persistRoundtrip(persistCtx, sessionID, registry, outcome)
	}

	result, err := loop.Run(ctx, req, onEvent, onRoundtrip)

	if pdfAttachment != nil {
		if deleteErr := attachment.Delete(service.dataDir, *pdfAttachment); deleteErr != nil {
			log.Printf("agent: delete pdf attachment for session %s: %v", sessionID, deleteErr)
		}
	}

	if imageAttachment != nil {
		if deleteErr := attachment.Delete(service.dataDir, *imageAttachment); deleteErr != nil {
			log.Printf("agent: delete image attachment for session %s: %v", sessionID, deleteErr)
		}
	}

	if err != nil {
		service.publishError(sessionID, err)
		return
	}

	// tokenCount == 0 only happens when "stop" (spec 01-01) cut the turn
	// off before any roundtrip's ChatEventDone ever reported real usage —
	// UpdateTokenCount SETs (not adds) the session's estimate from the
	// latest turn's own reported total (each turn resends the full
	// history, so its input_tokens already reflects the whole
	// conversation so far); writing 0 here would wipe out a real prior
	// estimate for no reason other than this particular turn never got far
	// enough to measure anything.
	if tokenCount := result.Usage.InputTokens + result.Usage.OutputTokens; tokenCount > 0 {
		if updateErr := chatRepo.UpdateTokenCount(persistCtx, sessionID, tokenCount); updateErr != nil {
			log.Printf("agent: update token count for session %s: %v", sessionID, updateErr)
		}
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
func (service *AgentService) persistRoundtrip(ctx context.Context, sessionID uuid.UUID, registry *agent.Registry, outcome agent.RoundtripOutcome) {
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

	messages := make([]*domain.ChatMessage, 0, 1+len(outcome.Results))
	messages = append(messages, assistantMessage)
	for _, toolOutcome := range outcome.Results {
		toolName := toolOutcome.Call.Name
		messages = append(messages, &domain.ChatMessage{
			ID:         uuid.New(),
			SessionID:  sessionID,
			Role:       domain.ChatRoleTool,
			Content:    humanizeToolResult(toolName, toolOutcome.Result, toolOutcome.IsError),
			ToolResult: toolResultJSON(toolOutcome.Call.ID, toolOutcome.Result),
			ToolName:   &toolName,
			CreatedAt:  time.Now().UTC(),
		})
	}

	// One transaction for the assistant row and all its role=tool rows: a
	// crash or a failed INSERT in between must never leave a persisted
	// tool_use without its tool_result (docs/buglist.md "dangling
	// tool_use"). All or nothing; RepairToolPairing covers rows that were
	// written before this was atomic.
	txErr := db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		txRepo := repository.NewChatRepo(tx)
		for _, message := range messages {
			if createErr := txRepo.CreateMessage(ctx, message); createErr != nil {
				return createErr
			}
		}
		return nil
	})
	if txErr != nil {
		log.Printf("agent: persist roundtrip for session %s: %v", sessionID, txErr)
		return
	}
	if isFinal {
		service.broker.Publish("chat.message", chatMessagePayload{SessionID: sessionID, Message: assistantMessage})
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
			calls = append(calls, agent.StoredToolCall{ID: block.ToolUseID, Name: block.ToolName, Input: validJSONOrEmptyObject(block.ToolInput)})
			seen[block.ToolUseID] = true
		}
	}
	for _, outcome := range results {
		if !seen[outcome.Call.ID] {
			calls = append(calls, agent.StoredToolCall{ID: outcome.Call.ID, Name: outcome.Call.Name, Input: validJSONOrEmptyObject(outcome.Call.Arguments)})
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

// validJSONOrEmptyObject returns raw if it is valid JSON, else "{}".
// json.Marshal of a StoredToolCall with an invalid json.RawMessage Input
// fails, and toolCallsJSON would then store no tool_calls at all while the
// role=tool rows are still written — an orphan tool_result on replay. A
// tool_use input is cut off mid-JSON when the model hits its output limit.
func validJSONOrEmptyObject(raw json.RawMessage) json.RawMessage {
	if json.Valid(raw) {
		return raw
	}
	return json.RawMessage("{}")
}

// toolResultJSON builds chat_message.tool_result's
// {"tool_call_id":"...","result":...} shape (ADR 0010). result is spliced
// in raw when it is valid JSON — every Registry tool implementation returns
// it via json.Marshal. If it is not (empty, truncated), it is stored as a
// JSON string instead: the column is JSONB, so an invalid value would make
// the INSERT fail and leave the tool_use without a stored result.
func toolResultJSON(callID, result string) json.RawMessage {
	idJSON, _ := json.Marshal(callID) //nolint:errchkjson // a string always marshals
	resultJSON := json.RawMessage(result)
	if !json.Valid(resultJSON) {
		resultJSON, _ = json.Marshal(result) //nolint:errchkjson // a string always marshals
	}
	return json.RawMessage(`{"tool_call_id":` + string(idJSON) + `,"result":` + string(resultJSON) + `}`)
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

// currentListingID returns the listing_id of the most recent message in
// history that has one — the Agent's notion of "what Listing are we
// currently discussing" (ADR 0016, Layer 8.2), used to inject that
// Listing's confirmed-fields block. Unlike attachedPDFPath, this scans the
// whole history backward rather than just the last row: a Listing's
// listing_id is only ever set on an assistant message that touched it via
// save_listing/download_image (agent.Registry.TouchedListingID), not on
// the triggering user message itself. nil once the session has never
// touched a Listing.
func currentListingID(history []domain.ChatMessage) *uuid.UUID {
	for _, message := range slices.Backward(history) {
		if message.ListingID != nil {
			return message.ListingID
		}
	}
	return nil
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

// attachedImagePath returns the triggering user message's attachment_path
// and the Claude MIME type to send it as if it names a JPG or PNG image,
// or (nil, "") otherwise. Mirrors attachedPDFPath above — same "only the
// last history row, chronological order" reasoning applies.
func attachedImagePath(history []domain.ChatMessage) (path *string, mediaType string) {
	if len(history) == 0 {
		return nil, ""
	}
	last := history[len(history)-1]
	if last.Role != domain.ChatRoleUser || last.AttachmentPath == nil {
		return nil, ""
	}
	lower := strings.ToLower(*last.AttachmentPath)
	switch {
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return last.AttachmentPath, "image/jpeg"
	case strings.HasSuffix(lower, ".png"):
		return last.AttachmentPath, "image/png"
	default:
		return nil, ""
	}
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
