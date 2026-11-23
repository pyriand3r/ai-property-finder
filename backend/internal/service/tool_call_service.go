package service

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
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
//
// Exception (docs/adr/0024): propose_criteria action=add has already stored
// a "proposed" criterion when the tool ran. Confirm approves that row
// instead of creating one; Reject deletes it. Proposals the rejection
// analysis wrote straight into chat_message.tool_calls have no such row, so
// for them Confirm still creates the (active) criterion.
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
// array and records "confirmed" on it (ADR 0017). Returns the entity that
// was written (*domain.Criterion or *domain.Context) — the handler
// serializes whichever it gets back.
func (service *ToolCallService) Confirm(ctx context.Context, messageID uuid.UUID, callIndex int) (any, error) {
	message, calls, call, err := service.loadToolCall(ctx, messageID, callIndex)
	if err != nil {
		return nil, err
	}

	var result any
	switch call.Name {
	case "propose_criteria":
		result, err = service.confirmProposeCriteria(ctx, message, call)
	case "update_context":
		result, err = service.confirmUpdateContext(ctx, message, call)
	default:
		return nil, &domain.ValidationError{Field: "tool", Message: "Tool-Call ist kein Vorschlag mit Bestätigung: " + call.Name}
	}
	if err != nil {
		return nil, err
	}

	if statusErr := service.markToolCallStatus(ctx, message, calls, callIndex, "confirmed"); statusErr != nil {
		return nil, statusErr
	}
	return result, nil
}

// Reject validates that callIndex names a confirmable, still-pending
// proposal and records "rejected" on it (ADR 0017) — no other state
// change; per ADR 0009 the rejection itself is just not confirming.
func (service *ToolCallService) Reject(ctx context.Context, messageID uuid.UUID, callIndex int) error {
	message, calls, call, err := service.loadToolCall(ctx, messageID, callIndex)
	if err != nil {
		return err
	}
	if call.Name != "propose_criteria" && call.Name != "update_context" {
		return &domain.ValidationError{Field: "tool", Message: "Tool-Call ist kein Vorschlag mit Bestätigung: " + call.Name}
	}
	if call.Name == "propose_criteria" {
		if discardError := service.discardProposedCriterion(ctx, message, call); discardError != nil {
			return discardError
		}
	}
	return service.markToolCallStatus(ctx, message, calls, callIndex, "rejected")
}

// loadToolCall reads messageID's tool_calls array and returns the whole
// parsed array (so a caller can mutate and rewrite it) alongside the
// requested entry. Returns 409 if that entry was already confirmed or
// rejected — confirm/reject are idempotent from the caller's perspective,
// never silently re-applying an append or no-op'ing a second reject.
func (service *ToolCallService) loadToolCall(ctx context.Context, messageID uuid.UUID, callIndex int) (message *domain.ChatMessage, calls []agent.StoredToolCall, call agent.StoredToolCall, err error) {
	message, err = repository.NewChatRepo(service.pool).GetMessage(ctx, messageID)
	if err != nil {
		return nil, nil, agent.StoredToolCall{}, err
	}
	if message.Role != domain.ChatRoleAssistant || len(message.ToolCalls) == 0 {
		return nil, nil, agent.StoredToolCall{}, &domain.ValidationError{Field: "message_id", Message: "Nachricht hat keine Tool-Calls"}
	}
	if unmarshalErr := json.Unmarshal(message.ToolCalls, &calls); unmarshalErr != nil {
		return nil, nil, agent.StoredToolCall{}, &domain.ValidationError{Field: "message_id", Message: "Tool-Calls konnten nicht gelesen werden"}
	}
	if callIndex < 0 || callIndex >= len(calls) {
		return nil, nil, agent.StoredToolCall{}, &domain.NotFoundError{Entity: "tool_call", ID: fmt.Sprintf("%s[%d]", messageID, callIndex)}
	}
	call = calls[callIndex]
	if call.Status != "" {
		return nil, nil, agent.StoredToolCall{}, &domain.ConflictError{Message: "Vorschlag wurde bereits " + statusVerbGerman(call.Status)}
	}
	return message, calls, call, nil
}

func statusVerbGerman(status string) string {
	if status == "confirmed" {
		return "bestätigt"
	}
	return "verworfen"
}

// markToolCallStatus writes status onto calls[callIndex] and persists the
// whole array back — the only field ever mutated post-insert on an
// otherwise immutable tool_calls row (ADR 0017).
func (service *ToolCallService) markToolCallStatus(ctx context.Context, message *domain.ChatMessage, calls []agent.StoredToolCall, callIndex int, status string) error {
	calls[callIndex].Status = status
	updated, err := json.Marshal(calls)
	if err != nil {
		return fmt.Errorf("marshal tool_calls: %w", err)
	}
	return repository.NewChatRepo(service.pool).UpdateToolCalls(ctx, message.ID, updated)
}

// criteriaProposal is the parsed input of a stored propose_criteria call.
type criteriaProposal struct {
	Action       string  `json:"action"`
	CriterionID  *string `json:"criterion_id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	IsSubjective bool    `json:"is_subjective"`
	Weight       *int    `json:"weight"`
}

// parseCriteriaProposal decodes a propose_criteria input and undoes HTML
// entity escaping in Name and Description (docs/buglist.md "Criterion names
// show literal &amp;", issue #4). Nothing in this backend escapes text: the
// model itself sometimes writes `Grundstück &amp; Lage` into a tool argument,
// and that string used to be stored as it was. Criterion text is plain text;
// escaping belongs at render time only, so decode here, on the one path where
// model-written text becomes a criterion row.
func parseCriteriaProposal(raw json.RawMessage) (criteriaProposal, error) {
	var args criteriaProposal
	if err := json.Unmarshal(raw, &args); err != nil {
		return criteriaProposal{}, err
	}
	args.Name = html.UnescapeString(args.Name)
	args.Description = html.UnescapeString(args.Description)
	return args, nil
}

func (service *ToolCallService) confirmProposeCriteria(ctx context.Context, message *domain.ChatMessage, call agent.StoredToolCall) (*domain.Criterion, error) {
	args, err := parseCriteriaProposal(call.Input)
	if err != nil {
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
		existing, err := service.findCriterionByName(ctx, session.ProfileID, args.Name)
		if err != nil {
			return nil, err
		}
		switch {
		case existing == nil:
			return service.criteria.Create(ctx, session.ProfileID, args.Name, args.Description, weight, args.IsSubjective)
		case existing.Status == domain.CriterionStatusProposed:
			return service.criteria.Approve(ctx, existing.ID)
		default:
			return nil, &domain.ConflictError{Message: "Vorschlag veraltet — Kriterium existiert bereits: " + args.Name}
		}
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

// findCriterionByName returns the profile's criterion with the given name
// (case-insensitive; names are unique per profile, docs/adr/0024), or nil.
func (service *ToolCallService) findCriterionByName(ctx context.Context, profileID uuid.UUID, name string) (*domain.Criterion, error) {
	existing, err := repository.NewCriterionRepo(service.pool).ListByProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	for i := range existing {
		if strings.EqualFold(existing[i].Name, name) {
			return &existing[i], nil
		}
	}
	return nil, nil //nolint:nilnil // nil means "no such criterion", not an error.
}

// discardProposedCriterion deletes the proposed criterion an action=add
// call stored, if it is still proposed. An edit, an already approved
// criterion, or a row the user deleted meanwhile leaves nothing to discard.
func (service *ToolCallService) discardProposedCriterion(ctx context.Context, message *domain.ChatMessage, call agent.StoredToolCall) error {
	args, err := parseCriteriaProposal(call.Input)
	if err != nil || args.Action != "add" {
		return nil //nolint:nilerr // an unreadable or non-add proposal has no stored row.
	}
	session, err := repository.NewChatRepo(service.pool).GetSession(ctx, message.SessionID)
	if err != nil {
		return err
	}
	existing, err := service.findCriterionByName(ctx, session.ProfileID, args.Name)
	if err != nil || existing == nil || existing.Status != domain.CriterionStatusProposed {
		return err
	}
	return service.criteria.Delete(ctx, existing.ID)
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
		return service.context.UpdateGlobalFromAgent(ctx, AppendContent(current.ContentMD, args.Content), message.ID)
	case "profile":
		session, err := repository.NewChatRepo(service.pool).GetSession(ctx, message.SessionID)
		if err != nil {
			return nil, err
		}
		current, err := service.context.GetProfileContext(ctx, session.ProfileID)
		if err != nil {
			return nil, err
		}
		return service.context.UpdateProfileContextFromAgent(ctx, session.ProfileID, AppendContent(current.ContentMD, args.Content), message.ID)
	default:
		return nil, &domain.ValidationError{Field: "scope", Message: "unbekannter scope: " + args.Scope}
	}
}

// AppendContent joins current and addition with a blank-line separator,
// or returns addition alone if current is blank. Shared by
// confirmUpdateContext's chat-confirm-card path above and
// internal/mcpserver's update_context tool (docs/adr/0019), which must
// produce byte-for-byte the same joined result a chat-confirmed
// update_context proposal would.
func AppendContent(current, addition string) string {
	if strings.TrimSpace(current) == "" {
		return addition
	}
	return current + "\n\n" + addition
}
