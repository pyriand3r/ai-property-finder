package service

import (
	"context"
	"encoding/json"
	"fmt"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/ai"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/events"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RejectionAnalysisService runs the single-shot "Rejection Feedback,
// Reimagined" operation (spec 01-04, Layer 8.1): when a Listing's status
// changes to rejected, analyze the rejection comment for a durable
// preference not yet captured, and — if the model actually finds one —
// write a synthetic assistant chat_message carrying propose_criteria/
// update_context proposals, exactly the shape a live Agent turn would have
// produced, so the existing Confirm-Card UI (ADR 0009/0017) picks it up
// unmodified. No Agent loop, no tools — one LLM call, same pattern as
// EvaluationService/RankingService.
type RejectionAnalysisService struct {
	pool     *pgxpool.Pool
	provider ai.Provider
	broker   *events.Broker
}

// NewRejectionAnalysisService constructs a RejectionAnalysisService wired
// to pool, provider, and broker (SSE toast notification).
func NewRejectionAnalysisService(pool *pgxpool.Pool, provider ai.Provider, broker *events.Broker) *RejectionAnalysisService {
	return &RejectionAnalysisService{pool: pool, provider: provider, broker: broker}
}

// AnalyzeRejection loads listingID (must currently be rejected, with a
// non-empty rejection_comment — the caller is expected to have already
// checked this before calling TriggerAsync, but it's re-checked here too
// since this is also directly unit-testable/callable on its own), asks the
// Provider whether the comment reveals a durable preference, and — only if
// it does — writes one new assistant chat_message onto the profile's
// active session and publishes chat.message (so an open Chat tab live-
// updates, same event the real Agent loop publishes) plus
// listing.rejection_analyzed (drives the toast, spec 01-04: „Rejection
// verarbeitet — Vorschläge im Chat verfügbar.").
func (service *RejectionAnalysisService) AnalyzeRejection(ctx context.Context, listingID uuid.UUID) error {
	listing, err := repository.NewListingRepo(service.pool).GetByID(ctx, listingID)
	if err != nil {
		return err
	}
	if listing.Status != domain.ListingStatusRejected {
		return nil
	}
	if listing.RejectionComment == nil || strings.TrimSpace(*listing.RejectionComment) == "" {
		return nil
	}

	criteria, err := repository.NewCriterionRepo(service.pool).ListByProfile(ctx, listing.ProfileID)
	if err != nil {
		return err
	}
	profileContext, err := repository.NewContextRepo(service.pool).GetByProfileID(ctx, listing.ProfileID)
	if err != nil {
		return err
	}

	result, err := service.provider.AnalyzeRejection(ctx, *listing, *listing.RejectionComment, criteria, profileContext.ContentMD)
	if err != nil {
		return mapProviderError(err)
	}
	if len(result.CriteriaProposals) == 0 && len(result.ContextProposals) == 0 {
		return nil
	}

	toolCalls, err := buildProposalToolCalls(result)
	if err != nil {
		return err
	}
	toolCallsJSON, err := json.Marshal(toolCalls)
	if err != nil {
		return fmt.Errorf("marshal rejection analysis tool calls: %w", err)
	}

	chatRepo := repository.NewChatRepo(service.pool)
	session, err := chatRepo.GetActiveSession(ctx, listing.ProfileID)
	if err != nil {
		return err
	}

	message := &domain.ChatMessage{
		ID:        uuid.New(),
		SessionID: session.ID,
		Role:      domain.ChatRoleAssistant,
		Content:   rejectionAnalysisSummary(listing.Title, len(result.CriteriaProposals), len(result.ContextProposals)),
		ToolCalls: toolCallsJSON,
		ListingID: &listing.ID,
		CreatedAt: time.Now().UTC(),
	}
	if createError := chatRepo.CreateMessage(ctx, message); createError != nil {
		return createError
	}

	service.broker.Publish("chat.message", map[string]any{"session_id": session.ID})
	service.broker.Publish("listing.rejection_analyzed", map[string]any{
		"profile_id": listing.ProfileID,
		"listing_id": listing.ID,
		"message_id": message.ID,
	})
	return nil
}

// buildProposalToolCalls turns AnalyzeRejection's raw proposals into the
// same agent.StoredToolCall shape a live Agent turn's propose_criteria/
// update_context calls would produce — Input matches those tools'
// InputSchema exactly (agent/tools.go) so ToolCallService.Confirm/Reject
// (ADR 0009) handles them with zero changes.
func buildProposalToolCalls(result ai.RejectionAnalysisResult) ([]agent.StoredToolCall, error) {
	calls := make([]agent.StoredToolCall, 0, len(result.CriteriaProposals)+len(result.ContextProposals))

	for _, proposal := range result.CriteriaProposals {
		input, err := json.Marshal(struct {
			Action       string  `json:"action"`
			CriterionID  *string `json:"criterion_id,omitempty"`
			Name         string  `json:"name"`
			Description  string  `json:"description"`
			IsSubjective bool    `json:"is_subjective"`
			Weight       *int    `json:"weight,omitempty"`
		}{
			Action:       proposal.Action,
			CriterionID:  proposal.CriterionID,
			Name:         proposal.Name,
			Description:  proposal.Description,
			IsSubjective: proposal.IsSubjective,
			Weight:       proposal.Weight,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal propose_criteria input: %w", err)
		}
		calls = append(calls, agent.StoredToolCall{ID: uuid.NewString(), Name: "propose_criteria", Input: input})
	}

	for _, proposal := range result.ContextProposals {
		input, err := json.Marshal(struct {
			Scope     string `json:"scope"`
			Operation string `json:"operation"`
			Content   string `json:"content"`
		}{
			Scope:     proposal.Scope,
			Operation: proposal.Operation,
			Content:   proposal.Content,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal update_context input: %w", err)
		}
		calls = append(calls, agent.StoredToolCall{ID: uuid.NewString(), Name: "update_context", Input: input})
	}

	return calls, nil
}

func rejectionAnalysisSummary(listingTitle string, criteriaCount, contextCount int) string {
	var parts []string
	if criteriaCount > 0 {
		parts = append(parts, fmt.Sprintf("%d Kriterien-Vorschlag/Vorschläge", criteriaCount))
	}
	if contextCount > 0 {
		parts = append(parts, fmt.Sprintf("%d Kontext-Vorschlag/Vorschläge", contextCount))
	}
	return fmt.Sprintf(
		"Ich habe die Ablehnung von „%s” analysiert und %s erstellt — bitte prüfen und bestätigen.",
		listingTitle, strings.Join(parts, " sowie "),
	)
}
