package service

import (
	"context"
	"encoding/json"
	"fmt"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ToolCallService applies or discards a pending propose_criteria
// /update_context proposal (ADR 0009/0004, Layer 6.5). The proposal itself
// was never written by the Agent's tool call — it lives verbatim in
// chat_message.tool_calls[callIndex].input, put there when the turn was
// persisted (spec 00-02). Confirm reads it from there, validates against
// current state, and writes; Reject only validates that the call exists.
type ToolCallService struct {
	pool     *pgxpool.Pool
	criteria *CriterionService
	context  *ContextService
}

// NewToolCallService constructs a ToolCallService wired to pool.
func NewToolCallService(pool *pgxpool.Pool) *ToolCallService {
	return &ToolCallService{pool: pool, criteria: NewCriterionService(pool), context: NewContextService(pool)}
}

// Confirm applies the tool call at callIndex within messageID's tool_calls
// array. Returns the entity that was written (*domain.Criterion or
// *domain.Context) — the handler serializes whichever it gets back.
func (service *ToolCallService) Confirm(ctx context.Context, messageID uuid.UUID, callIndex int) (any, error) {
	message, call, err := service.loadToolCall(ctx, messageID, callIndex)
	if err != nil {
		return nil, err
	}
	switch call.Name {
	case "propose_criteria":
		return service.confirmProposeCriteria(ctx, message, call)
	case "update_context":
		return service.confirmUpdateContext(ctx, message, call)
	default:
		return nil, &domain.ValidationError{Field: "tool", Message: "Tool-Call ist kein Vorschlag mit Bestätigung: " + call.Name}
	}
}

// Reject validates that callIndex names a confirmable proposal and
// otherwise does nothing — per ADR 0009, rejection is implicit in not
// confirming; this endpoint exists so the frontend has a clean call to
// dismiss the card.
func (service *ToolCallService) Reject(ctx context.Context, messageID uuid.UUID, callIndex int) error {
	_, call, err := service.loadToolCall(ctx, messageID, callIndex)
	if err != nil {
		return err
	}
	if call.Name != "propose_criteria" && call.Name != "update_context" {
		return &domain.ValidationError{Field: "tool", Message: "Tool-Call ist kein Vorschlag mit Bestätigung: " + call.Name}
	}
	return nil
}

func (service *ToolCallService) loadToolCall(ctx context.Context, messageID uuid.UUID, callIndex int) (*domain.ChatMessage, agent.StoredToolCall, error) {
	message, err := repository.NewChatRepo(service.pool).GetMessage(ctx, messageID)
	if err != nil {
		return nil, agent.StoredToolCall{}, err
	}
	if message.Role != domain.ChatRoleAssistant || len(message.ToolCalls) == 0 {
		return nil, agent.StoredToolCall{}, &domain.ValidationError{Field: "message_id", Message: "Nachricht hat keine Tool-Calls"}
	}
	var calls []agent.StoredToolCall
	if unmarshalErr := json.Unmarshal(message.ToolCalls, &calls); unmarshalErr != nil {
		return nil, agent.StoredToolCall{}, &domain.ValidationError{Field: "message_id", Message: "Tool-Calls konnten nicht gelesen werden"}
	}
	if callIndex < 0 || callIndex >= len(calls) {
		return nil, agent.StoredToolCall{}, &domain.NotFoundError{Entity: "tool_call", ID: fmt.Sprintf("%s[%d]", messageID, callIndex)}
	}
	return message, calls[callIndex], nil
}

func (service *ToolCallService) confirmProposeCriteria(ctx context.Context, message *domain.ChatMessage, call agent.StoredToolCall) (*domain.Criterion, error) {
	var args struct {
		Action       string  `json:"action"`
		CriterionID  *string `json:"criterion_id"`
		Name         string  `json:"name"`
		Description  string  `json:"description"`
		IsSubjective bool    `json:"is_subjective"`
		Weight       *int    `json:"weight"`
	}
	if err := json.Unmarshal(call.Input, &args); err != nil {
		return nil, &domain.ValidationError{Field: "tool_call", Message: "Vorschlag konnte nicht gelesen werden"}
	}
	weight := 1
	if args.Weight != nil {
		weight = *args.Weight
	}

	session, err := repository.NewChatRepo(service.pool).GetSession(ctx, message.SessionID)
	if err != nil {
		return nil, err
	}

	switch args.Action {
	case "add":
		existing, err := repository.NewCriterionRepo(service.pool).ListByProfile(ctx, session.ProfileID)
		if err != nil {
			return nil, err
		}
		for _, criterion := range existing {
			if strings.EqualFold(criterion.Name, args.Name) {
				return nil, &domain.ConflictError{Message: "Vorschlag veraltet — Kriterium existiert bereits: " + args.Name}
			}
		}
		return service.criteria.Create(ctx, session.ProfileID, args.Name, args.Description, weight, args.IsSubjective)
	case "edit":
		if args.CriterionID == nil {
			return nil, &domain.ValidationError{Field: "criterion_id", Message: "erforderlich bei action=edit"}
		}
		criterionID, parseErr := uuid.Parse(*args.CriterionID)
		if parseErr != nil {
			return nil, &domain.ValidationError{Field: "criterion_id", Message: "ungültige UUID"}
		}
		name, description, isSubjective := args.Name, args.Description, args.IsSubjective
		return service.criteria.Update(ctx, criterionID, &name, &description, args.Weight, &isSubjective)
	default:
		return nil, &domain.ValidationError{Field: "action", Message: "unbekannte action: " + args.Action}
	}
}

// confirmUpdateContext always applies onto the *current* content_md,
// fetched fresh at confirm time — for an append-only operation (the only
// one supported) that can never lose data even if the base text changed
// since the proposal was made, so there's no separate staleness check to
// perform here (unlike propose_criteria's name-collision/existence
// checks, where applying onto stale state could silently duplicate or
// clobber something).
func (service *ToolCallService) confirmUpdateContext(ctx context.Context, message *domain.ChatMessage, call agent.StoredToolCall) (*domain.Context, error) {
	var args struct {
		Scope     string `json:"scope"`
		Operation string `json:"operation"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal(call.Input, &args); err != nil {
		return nil, &domain.ValidationError{Field: "tool_call", Message: "Vorschlag konnte nicht gelesen werden"}
	}
	if args.Operation != "append" {
		return nil, &domain.ValidationError{Field: "operation", Message: "nur 'append' wird unterstützt"}
	}

	switch args.Scope {
	case "global":
		current, err := service.context.GetGlobal(ctx)
		if err != nil {
			return nil, err
		}
		return service.context.UpdateGlobalFromAgent(ctx, appendContent(current.ContentMD, args.Content), message.ID)
	case "profile":
		session, err := repository.NewChatRepo(service.pool).GetSession(ctx, message.SessionID)
		if err != nil {
			return nil, err
		}
		current, err := service.context.GetProfileContext(ctx, session.ProfileID)
		if err != nil {
			return nil, err
		}
		return service.context.UpdateProfileContextFromAgent(ctx, session.ProfileID, appendContent(current.ContentMD, args.Content), message.ID)
	default:
		return nil, &domain.ValidationError{Field: "scope", Message: "unbekannter scope: " + args.Scope}
	}
}

func appendContent(current, addition string) string {
	if strings.TrimSpace(current) == "" {
		return addition
	}
	return current + "\n\n" + addition
}
