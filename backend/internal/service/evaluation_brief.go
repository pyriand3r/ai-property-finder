package service

import (
	"context"
	"fmt"
	"propertyfinder/internal/ai"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"strings"

	"github.com/google/uuid"
)

// evaluationResponseFormat is the answer shape an external grader sends
// back through submit_evaluation (docs/adr/0026).
const evaluationResponseFormat = `[{"criterion_id": "<uuid>", "grade": "A".."F", "reasoning": "..."}]`

// BriefCriterion is one active Criterion in an EvaluationBrief.
type BriefCriterion struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Weight       int       `json:"weight"`
	IsSubjective bool      `json:"is_subjective"`
}

// BriefListing is the Listing part of an EvaluationBrief. Text is the
// evaluation-relevant field rendering the internal evaluation prompt uses
// (ai.RenderListing); DocumentMD is the Agent-written Listing Document.
type BriefListing struct {
	ID         uuid.UUID `json:"id"`
	Text       string    `json:"text"`
	DocumentMD *string   `json:"document_md,omitempty"`
}

// EvaluationBrief is everything an external client needs to grade one
// Listing itself (docs/adr/0026, spec 01-14): the grading instruction, the
// active Criteria, the Listing, both Context Files and the manually
// confirmed fields (ADR 0016).
type EvaluationBrief struct {
	Instruction         string           `json:"instruction"`
	Criteria            []BriefCriterion `json:"criteria"`
	Listing             BriefListing     `json:"listing"`
	GlobalContext       string           `json:"global_context"`
	ProfileContext      string           `json:"profile_context"`
	UserConfirmedFields []string         `json:"user_confirmed_fields"`
	ResponseFormat      string           `json:"response_format"`
}

// SubmittedGrade is one grade as an external client sends it: the raw
// values, validated by SubmitGrades.
type SubmittedGrade struct {
	CriterionID string `json:"criterion_id"`
	Grade       string `json:"grade"`
	Reasoning   string `json:"reasoning"`
}

// activeCriteria loads listingID's listing and its profile's active
// Criteria; no active Criteria is the same ValidationError for every
// evaluation path.
func (service *EvaluationService) activeCriteria(ctx context.Context, listingID uuid.UUID) (*domain.Listing, []domain.Criterion, error) {
	listing, err := repository.NewListingRepo(service.pool).GetByID(ctx, listingID)
	if err != nil {
		return nil, nil, err
	}
	// Active criteria only: a proposed one is not part of the evaluation
	// until the user approves it (docs/adr/0024).
	criteria, err := repository.NewCriterionRepo(service.pool).ListActiveByProfile(ctx, listing.ProfileID)
	if err != nil {
		return nil, nil, err
	}
	if len(criteria) == 0 {
		return nil, nil, &domain.ValidationError{Field: "criteria", Message: "Profil hat keine aktiven Kriterien — zuerst Kriterien anlegen oder freigeben"}
	}
	return listing, criteria, nil
}

// Brief assembles the EvaluationBrief for listingID. Proposed Criteria are
// not part of it.
func (service *EvaluationService) Brief(ctx context.Context, listingID uuid.UUID) (*EvaluationBrief, error) {
	listing, criteria, err := service.activeCriteria(ctx, listingID)
	if err != nil {
		return nil, err
	}
	contextRepo := repository.NewContextRepo(service.pool)
	globalContext, err := contextRepo.GetGlobal(ctx)
	if err != nil {
		return nil, err
	}
	profileContext, err := contextRepo.GetByProfileID(ctx, listing.ProfileID)
	if err != nil {
		return nil, err
	}

	brief := &EvaluationBrief{
		Instruction:         ai.EvaluationInstruction(ai.EvaluationByCriterionID),
		Criteria:            make([]BriefCriterion, len(criteria)),
		Listing:             BriefListing{ID: listing.ID, Text: ai.RenderListing(*listing), DocumentMD: listing.DocumentMD},
		GlobalContext:       globalContext.ContentMD,
		ProfileContext:      profileContext.ContentMD,
		UserConfirmedFields: listing.UserConfirmedFields,
		ResponseFormat:      evaluationResponseFormat,
	}
	if brief.UserConfirmedFields == nil {
		brief.UserConfirmedFields = []string{}
	}
	for i, criterion := range criteria {
		brief.Criteria[i] = BriefCriterion{
			ID:           criterion.ID,
			Name:         criterion.Name,
			Description:  criterion.Description,
			Weight:       criterion.Weight,
			IsSubjective: criterion.IsSubjective,
		}
	}
	return brief, nil
}

// SubmitGrades validates grades from an external client and stores them via
// ApplyGrades (docs/adr/0026). Every active Criterion of the listing's
// profile must be graded exactly once; an unknown, duplicate or missing
// criterion_id, a grade outside A-F, or an empty reasoning is a
// ValidationError and nothing is written. The returned Grades are in the
// order of the active Criteria; the Score is computed by the server.
func (service *EvaluationService) SubmitGrades(ctx context.Context, listingID uuid.UUID, grades []SubmittedGrade) (*EvaluationResult, error) {
	_, criteria, err := service.activeCriteria(ctx, listingID)
	if err != nil {
		return nil, err
	}

	byID := make(map[uuid.UUID]int, len(criteria))
	for i, criterion := range criteria {
		byID[criterion.ID] = i
	}
	scores := make([]domain.CriterionScore, len(criteria))
	seen := make([]bool, len(criteria))
	for _, submitted := range grades {
		criterionID, parseError := uuid.Parse(strings.TrimSpace(submitted.CriterionID))
		if parseError != nil {
			return nil, &domain.ValidationError{Field: "grades", Message: fmt.Sprintf("criterion_id %q ist keine gültige UUID", submitted.CriterionID)}
		}
		index, known := byID[criterionID]
		if !known {
			return nil, &domain.ValidationError{Field: "grades", Message: fmt.Sprintf("unbekannte criterion_id %s (kein aktives Kriterium dieses Profils)", criterionID)}
		}
		if seen[index] {
			return nil, &domain.ValidationError{Field: "grades", Message: fmt.Sprintf("doppelte criterion_id %s", criterionID)}
		}
		grade, gradeError := ai.NormalizeGrade(submitted.Grade)
		if gradeError != nil {
			return nil, &domain.ValidationError{Field: "grades", Message: fmt.Sprintf("criterion_id %s: ungültige Note %q (erlaubt: A-F)", criterionID, submitted.Grade)}
		}
		reasoning := strings.TrimSpace(submitted.Reasoning)
		if reasoning == "" {
			return nil, &domain.ValidationError{Field: "grades", Message: fmt.Sprintf("criterion_id %s: reasoning darf nicht leer sein", criterionID)}
		}
		seen[index] = true
		scores[index] = domain.CriterionScore{Index: index, CriterionID: criterionID, Grade: grade, Reasoning: reasoning}
	}
	var missing []string
	for i, criterion := range criteria {
		if !seen[i] {
			missing = append(missing, fmt.Sprintf("%s (%s)", criterion.ID, criterion.Name))
		}
	}
	if len(missing) > 0 {
		return nil, &domain.ValidationError{Field: "grades", Message: "fehlende criterion_id: " + strings.Join(missing, ", ")}
	}

	if applyError := service.ApplyGrades(ctx, listingID, scores); applyError != nil {
		return nil, applyError
	}
	updated, err := repository.NewListingRepo(service.pool).GetByID(ctx, listingID)
	if err != nil {
		return nil, err
	}
	return &EvaluationResult{Listing: updated, Grades: scores}, nil
}

// ListPending returns the listings of profileID flagged needs_reevaluation
// (the same filter ReevaluateAll uses).
func (service *EvaluationService) ListPending(ctx context.Context, profileID uuid.UUID) ([]domain.Listing, error) {
	stale := true
	return repository.NewListingRepo(service.pool).ListByProfile(ctx, profileID, &stale)
}
