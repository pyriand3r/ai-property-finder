package mcpserver

import (
	"context"
	"fmt"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListProfilesInput takes no fields — list_profiles is the bootstrap tool
// an MCP client calls first to discover valid profile_id values for every
// other tool in this catalog.
type ListProfilesInput struct{}

// ProfileOutput mirrors domain.SearchProfile with ID rendered as a plain
// string. uuid.UUID is a [16]byte array under the hood; jsonschema-go's
// reflection doesn't special-case its custom MarshalJSON the way it does
// for stdlib types like time.Time, so a raw domain.SearchProfile in an Out
// position produces an output schema requiring an integer array for "id"
// while the real marshaled value is a string — every tools/call then
// fails output-schema validation. Rendering IDs as strings up front in a
// dedicated output DTO sidesteps that reflection gap entirely.
type ProfileOutput struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	RankingSummary *string    `json:"ranking_summary,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func newProfileOutput(profile domain.SearchProfile) ProfileOutput {
	return ProfileOutput{
		ID:             profile.ID.String(),
		Name:           profile.Name,
		Description:    profile.Description,
		ArchivedAt:     profile.ArchivedAt,
		RankingSummary: profile.RankingSummary,
		CreatedAt:      profile.CreatedAt,
		UpdatedAt:      profile.UpdatedAt,
	}
}

// ListProfilesOutput wraps the profile list under a named key so it
// serializes as a JSON object (required for the reflected output schema),
// not a bare array.
type ListProfilesOutput struct {
	Profiles []ProfileOutput `json:"profiles"`
}

func listProfilesHandler(deps Deps) mcp.ToolHandlerFor[ListProfilesInput, ListProfilesOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ ListProfilesInput) (*mcp.CallToolResult, ListProfilesOutput, error) {
		profiles, err := deps.Profiles.List(ctx)
		if err != nil {
			return nil, ListProfilesOutput{}, err
		}
		out := make([]ProfileOutput, len(profiles))
		for i, profile := range profiles {
			out[i] = newProfileOutput(profile)
		}
		return nil, ListProfilesOutput{Profiles: out}, nil
	}
}

// CriterionProposalInput mirrors propose_criteria's schema in
// internal/agent/tools.go. Unlike the internal chat Agent's identically
// named Tool, this one has no Confirm-Card (meaningless outside a real
// chat_message row): it stores the criterion with status "proposed" right
// away and can only add or refine proposals. An active criterion cannot be
// changed through MCP — the user approves proposals in the Criteria tab
// (docs/adr/0024, partly replacing docs/adr/0019 for criteria).
type CriterionProposalInput struct {
	ProfileID    string  `json:"profile_id" jsonschema:"UUID of the Search Profile"`
	Action       string  `json:"action" jsonschema:"add or edit; edit only works on a criterion that is still proposed"`
	CriterionID  *string `json:"criterion_id,omitempty" jsonschema:"UUID of the proposed criterion to edit; required when action=edit"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	IsSubjective bool    `json:"is_subjective,omitempty" jsonschema:"true = subjective (LLM judgement only), false = objective (read directly from data)"`
	Weight       *int    `json:"weight,omitempty" jsonschema:"1-5, default 1"`
}

// CriterionOutput mirrors domain.Criterion with ID/ProfileID rendered as
// plain strings — see ProfileOutput's doc comment for why.
type CriterionOutput struct {
	ID           string    `json:"id"`
	ProfileID    string    `json:"profile_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Weight       int       `json:"weight"`
	IsSubjective bool      `json:"is_subjective"`
	SortOrder    int       `json:"sort_order"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func newCriterionOutput(criterion domain.Criterion) CriterionOutput {
	return CriterionOutput{
		ID:           criterion.ID.String(),
		ProfileID:    criterion.ProfileID.String(),
		Name:         criterion.Name,
		Description:  criterion.Description,
		Weight:       criterion.Weight,
		IsSubjective: criterion.IsSubjective,
		SortOrder:    criterion.SortOrder,
		Status:       string(criterion.Status),
		CreatedAt:    criterion.CreatedAt,
		UpdatedAt:    criterion.UpdatedAt,
	}
}

func proposeCriteriaHandler(deps Deps) mcp.ToolHandlerFor[CriterionProposalInput, CriterionOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input CriterionProposalInput) (*mcp.CallToolResult, CriterionOutput, error) {
		profileID, err := uuid.Parse(input.ProfileID)
		if err != nil {
			return nil, CriterionOutput{}, fmt.Errorf("invalid profile_id: %w", err)
		}
		weight := 1
		if input.Weight != nil {
			weight = *input.Weight
		}
		switch input.Action {
		case "add":
			created, err := deps.Criteria.Propose(ctx, profileID, input.Name, input.Description, weight, input.IsSubjective)
			if err != nil {
				return nil, CriterionOutput{}, err
			}
			return nil, newCriterionOutput(*created), nil
		case "edit":
			if input.CriterionID == nil {
				return nil, CriterionOutput{}, fmt.Errorf("criterion_id ist erforderlich bei action=edit")
			}
			criterionID, err := uuid.Parse(*input.CriterionID)
			if err != nil {
				return nil, CriterionOutput{}, fmt.Errorf("invalid criterion_id: %w", err)
			}
			name, description, isSubjective := input.Name, input.Description, input.IsSubjective
			updated, err := deps.Criteria.UpdateProposed(ctx, criterionID, &name, &description, input.Weight, &isSubjective)
			if err != nil {
				return nil, CriterionOutput{}, err
			}
			return nil, newCriterionOutput(*updated), nil
		default:
			return nil, CriterionOutput{}, fmt.Errorf("unbekannte action: %s", input.Action)
		}
	}
}

// UpdateContextInput mirrors update_context's schema. Unlike
// propose_criteria above, this writes immediately via
// ContextService.UpdateGlobal/UpdateProfileContext (the same path the
// plain Context Editor form uses, ADR 0001) — recorded as a
// ChangedByUser revision, not ChangedByAgent, since there is no
// chat_message to attribute it to (docs/adr/0019).
type UpdateContextInput struct {
	ProfileID string `json:"profile_id" jsonschema:"UUID of the Search Profile (ignored when scope=global, but still required for schema uniformity)"`
	Scope     string `json:"scope" jsonschema:"global or profile"`
	Operation string `json:"operation" jsonschema:"Only 'append' is supported — content is appended to the existing text"`
	Content   string `json:"content" jsonschema:"Text to append"`
}

// ContextOutput mirrors domain.Context with ID/ProfileID rendered as
// plain strings — see ProfileOutput's doc comment for why.
type ContextOutput struct {
	ID        string    `json:"id"`
	Scope     string    `json:"scope"`
	ProfileID *string   `json:"profile_id,omitempty"`
	ContentMD string    `json:"content_md"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newContextOutput(ctxDoc domain.Context) ContextOutput {
	var profileID *string
	if ctxDoc.ProfileID != nil {
		id := ctxDoc.ProfileID.String()
		profileID = &id
	}
	return ContextOutput{
		ID:        ctxDoc.ID.String(),
		Scope:     string(ctxDoc.Scope),
		ProfileID: profileID,
		ContentMD: ctxDoc.ContentMD,
		CreatedAt: ctxDoc.CreatedAt,
		UpdatedAt: ctxDoc.UpdatedAt,
	}
}

func updateContextHandler(deps Deps) mcp.ToolHandlerFor[UpdateContextInput, ContextOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input UpdateContextInput) (*mcp.CallToolResult, ContextOutput, error) {
		if input.Operation != "append" {
			return nil, ContextOutput{}, fmt.Errorf("nur 'append' wird unterstützt")
		}
		switch input.Scope {
		case "global":
			current, err := deps.Context.GetGlobal(ctx)
			if err != nil {
				return nil, ContextOutput{}, err
			}
			updated, err := deps.Context.UpdateGlobal(ctx, service.AppendContent(current.ContentMD, input.Content))
			if err != nil {
				return nil, ContextOutput{}, err
			}
			return nil, newContextOutput(*updated), nil
		case "profile":
			profileID, err := uuid.Parse(input.ProfileID)
			if err != nil {
				return nil, ContextOutput{}, fmt.Errorf("invalid profile_id: %w", err)
			}
			current, err := deps.Context.GetProfileContext(ctx, profileID)
			if err != nil {
				return nil, ContextOutput{}, err
			}
			updated, err := deps.Context.UpdateProfileContext(ctx, profileID, service.AppendContent(current.ContentMD, input.Content))
			if err != nil {
				return nil, ContextOutput{}, err
			}
			return nil, newContextOutput(*updated), nil
		default:
			return nil, ContextOutput{}, fmt.Errorf("unbekannter scope: %s", input.Scope)
		}
	}
}
