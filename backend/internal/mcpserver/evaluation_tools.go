package mcpserver

import (
	"context"
	"fmt"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// External evaluation (docs/adr/0026, spec 01-14): the client grades a
// Listing itself. These tools call EvaluationService directly (like
// list_profiles), not the agent Registry, and need no LLM key. Outputs use
// Out=any: the domain/service structs carry uuid.UUID fields, which the
// reflected output schema would describe as arrays (see ProfileOutput).

// GetEvaluationBriefInput is the input of get_evaluation_brief.
type GetEvaluationBriefInput struct {
	ListingID string `json:"listing_id" jsonschema:"UUID of the Listing to grade"`
}

func getEvaluationBriefHandler(deps Deps) mcp.ToolHandlerFor[GetEvaluationBriefInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input GetEvaluationBriefInput) (*mcp.CallToolResult, any, error) {
		listingID, err := uuid.Parse(input.ListingID)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid listing_id: %w", err)
		}
		brief, err := deps.Evaluation.Brief(ctx, listingID)
		if err != nil {
			return nil, nil, err
		}
		return nil, brief, nil
	}
}

// GradeInput is one grade in submit_evaluation.
type GradeInput struct {
	CriterionID string `json:"criterion_id" jsonschema:"UUID of an active Criterion, copied from the brief"`
	Grade       string `json:"grade" jsonschema:"A (best) to F (worst)"`
	Reasoning   string `json:"reasoning" jsonschema:"Short explanation; must not be empty"`
}

// SubmitEvaluationInput is the input of submit_evaluation.
type SubmitEvaluationInput struct {
	ListingID string       `json:"listing_id" jsonschema:"UUID of the Listing that was graded"`
	Grades    []GradeInput `json:"grades" jsonschema:"Exactly one grade per active Criterion"`
}

// SubmitEvaluationOutput is the updated Listing (with the server-computed
// overall_score) and the stored Grades.
type SubmitEvaluationOutput struct {
	Listing *domain.Listing         `json:"listing"`
	Grades  []domain.CriterionScore `json:"grades"`
}

func submitEvaluationHandler(deps Deps) mcp.ToolHandlerFor[SubmitEvaluationInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input SubmitEvaluationInput) (*mcp.CallToolResult, any, error) {
		listingID, err := uuid.Parse(input.ListingID)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid listing_id: %w", err)
		}
		grades := make([]service.SubmittedGrade, len(input.Grades))
		for i, grade := range input.Grades {
			grades[i] = service.SubmittedGrade{CriterionID: grade.CriterionID, Grade: grade.Grade, Reasoning: grade.Reasoning}
		}
		result, err := deps.Evaluation.SubmitGrades(ctx, listingID, grades)
		if err != nil {
			return nil, nil, err
		}
		return nil, SubmitEvaluationOutput{Listing: result.Listing, Grades: result.Grades}, nil
	}
}

// ListPendingEvaluationsInput is the input of list_pending_evaluations.
type ListPendingEvaluationsInput struct {
	ProfileID string `json:"profile_id" jsonschema:"UUID of the Search Profile (call list_profiles first if unknown)"`
}

// PendingEvaluation is one Listing waiting for (re-)evaluation.
type PendingEvaluation struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	OverallScore *float64 `json:"overall_score"`
}

// ListPendingEvaluationsOutput wraps the list under a named key (object
// output, see ListProfilesOutput).
type ListPendingEvaluationsOutput struct {
	Listings []PendingEvaluation `json:"listings"`
}

func listPendingEvaluationsHandler(deps Deps) mcp.ToolHandlerFor[ListPendingEvaluationsInput, ListPendingEvaluationsOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input ListPendingEvaluationsInput) (*mcp.CallToolResult, ListPendingEvaluationsOutput, error) {
		profileID, err := uuid.Parse(input.ProfileID)
		if err != nil {
			return nil, ListPendingEvaluationsOutput{}, fmt.Errorf("invalid profile_id: %w", err)
		}
		listings, err := deps.Evaluation.ListPending(ctx, profileID)
		if err != nil {
			return nil, ListPendingEvaluationsOutput{}, err
		}
		out := make([]PendingEvaluation, len(listings))
		for i, listing := range listings {
			out[i] = PendingEvaluation{ID: listing.ID.String(), Title: listing.Title, Status: string(listing.Status), OverallScore: listing.OverallScore}
		}
		return nil, ListPendingEvaluationsOutput{Listings: out}, nil
	}
}
