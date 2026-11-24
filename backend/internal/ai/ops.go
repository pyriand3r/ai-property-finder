package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"propertyfinder/internal/domain"
	"strings"
)

// This file holds the evaluate_listing prompt template and response parser.
// The concrete provider implements only the transport (completer); the
// prompt design, JSON robustness, and position-based index mapping live
// here so every provider behaves identically (spec 00-03, ADR 0006).

// EvaluationFormat selects how a grader must address the criteria in its
// answer. The internal call maps grades back by position (ADR 0006); an
// external grader (docs/adr/0026) answers by criterion_id.
type EvaluationFormat int

const (
	// EvaluationByPosition: one object per criterion, same order as the
	// numbered list, identified by its 1-based index.
	EvaluationByPosition EvaluationFormat = iota
	// EvaluationByCriterionID: one object per criterion, identified by its
	// criterion_id; order is irrelevant.
	EvaluationByCriterionID
)

// evaluationBase is the format-independent part of the grading instruction.
const evaluationBase = `You grade a real-estate listing against an ordered, numbered list of criteria.

For EACH criterion, assign a letter grade A (best) to F (worst) and a short reasoning text. Grade only against the criterion's meaning; if the listing lacks the information a criterion needs, grade it low and say so.`

const evaluationFormatByPosition = `Return ONLY a JSON array with EXACTLY one object per criterion, in the SAME ORDER as the numbered list:
{"index": <1-based position>, "grade": "A".."F", "reasoning": "<short explanation>"}

The array length MUST equal the number of criteria. No prose, no markdown fences.`

const evaluationFormatByCriterionID = `Return ONLY a JSON array with EXACTLY one object per criterion, identified by the criterion's criterion_id (copy it exactly from the criteria list; the order does not matter):
{"criterion_id": "<criterion id>", "grade": "A".."F", "reasoning": "<short explanation>"}

The array MUST contain every criterion exactly once. No prose, no markdown fences.`

// EvaluationInstruction returns the grading instruction: the shared base
// text plus the answer-format part for format. It is the single source for
// the internal evaluate_listing prompt and the external evaluation brief.
func EvaluationInstruction(format EvaluationFormat) string {
	if format == EvaluationByCriterionID {
		return evaluationBase + "\n\n" + evaluationFormatByCriterionID
	}
	return evaluationBase + "\n\n" + evaluationFormatByPosition
}

// rawGrade is the wire shape for one evaluated criterion.
type rawGrade struct {
	Index     int    `json:"index"`
	Grade     string `json:"grade"`
	Reasoning string `json:"reasoning"`
}

// runEvaluate implements Provider.EvaluateListing against any completer.
// The backend assigns CriterionID by position — the model never sees or
// echoes a UUID (ADR 0006).
func runEvaluate(ctx context.Context, c completer, listing domain.Listing, criteria []domain.Criterion) ([]domain.CriterionScore, error) {
	if len(criteria) == 0 {
		return []domain.CriterionScore{}, nil
	}
	cfg := c.config()

	var user strings.Builder
	fmt.Fprintf(&user, "Reasoning language: %s.\n\n", cfg.reasoningLanguage())
	user.WriteString("Listing:\n")
	user.WriteString(renderListing(listing))
	user.WriteString("\n\nCriteria (grade every one, in order):\n")
	for i, criterion := range criteria {
		kind := "objective"
		if criterion.IsSubjective {
			kind = "subjective"
		}
		fmt.Fprintf(&user, "%d. [%s] %s — %s\n", i+1, kind, criterion.Name, criterion.Description)
	}

	return callWithRetries(ctx, c, EvaluationInstruction(EvaluationByPosition), user.String(), func(raw string) ([]domain.CriterionScore, error) {
		return parseGrades(raw, criteria)
	})
}

// parseGrades extracts exactly len(criteria) grade objects and maps them
// back to criterion IDs by position. A wrong-length array or an invalid
// grade is an ErrParse so the corrective retry fires.
func parseGrades(raw string, criteria []domain.Criterion) ([]domain.CriterionScore, error) {
	value, err := extractFirstJSON(raw)
	if err != nil {
		return nil, err
	}
	var wire []rawGrade
	if err := json.Unmarshal([]byte(value), &wire); err != nil {
		return nil, fmt.Errorf("unmarshal grades: %w", ErrParse)
	}
	if len(wire) != len(criteria) {
		return nil, fmt.Errorf("got %d grades, want %d: %w", len(wire), len(criteria), ErrParse)
	}
	out := make([]domain.CriterionScore, len(criteria))
	for i, w := range wire {
		grade, gradeErr := NormalizeGrade(w.Grade)
		if gradeErr != nil {
			return nil, gradeErr
		}
		out[i] = domain.CriterionScore{
			Index:       i,
			CriterionID: criteria[i].ID, // backend-assigned, never from the model
			Grade:       grade,
			Reasoning:   strings.TrimSpace(w.Reasoning),
		}
	}
	return out, nil
}

const rankingSystem = `You write short, comparative one-liners for a ranked list of real-estate listings, plus one overall summary.

You receive listings numbered by rank (1 = best Score), each with its Score and key facts. For EACH listing, write one short sentence (Kurzfazit) — punchy, comparative, mentioning what stands out versus the others (e.g. "Beste Infrastruktur, aber kleinstes Grundstück"). Then write one paragraph (summary) comparing the top few listings overall, as a buyer-facing recap.

Return ONLY a JSON object:
{"kurzfazits": ["<one per listing, SAME ORDER as numbered list>"], "summary": "<comparison paragraph>"}

The kurzfazits array length MUST equal the number of listings. No prose outside the JSON, no markdown fences.`

// rawRanking is the wire shape of GenerateRanking's response.
type rawRanking struct {
	Kurzfazits []string `json:"kurzfazits"`
	Summary    string   `json:"summary"`
}

// runGenerateRanking implements Provider.GenerateRanking against any
// completer. listings must already be sorted by Score desc — the model
// only narrates the order, it doesn't decide it.
func runGenerateRanking(ctx context.Context, c completer, listings []domain.Listing) (RankingResult, error) {
	if len(listings) == 0 {
		return RankingResult{}, nil
	}
	cfg := c.config()

	var user strings.Builder
	fmt.Fprintf(&user, "Reasoning language: %s.\n\n", cfg.reasoningLanguage())
	user.WriteString("Ranked listings (best Score first):\n")
	for i, listing := range listings {
		score := "n/a"
		if listing.OverallScore != nil {
			score = fmt.Sprintf("%.0f", *listing.OverallScore)
		}
		fmt.Fprintf(&user, "%d. [Score %s] %s\n", i+1, score, renderListing(listing))
	}

	return callWithRetries(ctx, c, rankingSystem, user.String(), func(raw string) (RankingResult, error) {
		return parseRanking(raw, len(listings))
	})
}

// parseRanking extracts exactly wantCount kurzfazits plus the summary. A
// wrong-length array is an ErrParse so the corrective retry fires.
func parseRanking(raw string, wantCount int) (RankingResult, error) {
	value, err := extractFirstJSON(raw)
	if err != nil {
		return RankingResult{}, err
	}
	var wire rawRanking
	if err := json.Unmarshal([]byte(value), &wire); err != nil {
		return RankingResult{}, fmt.Errorf("unmarshal ranking: %w", ErrParse)
	}
	if len(wire.Kurzfazits) != wantCount {
		return RankingResult{}, fmt.Errorf("got %d kurzfazits, want %d: %w", len(wire.Kurzfazits), wantCount, ErrParse)
	}
	if strings.TrimSpace(wire.Summary) == "" {
		return RankingResult{}, fmt.Errorf("empty summary: %w", ErrParse)
	}
	kurzfazits := make([]string, len(wire.Kurzfazits))
	for i, text := range wire.Kurzfazits {
		kurzfazits[i] = strings.TrimSpace(text)
	}
	return RankingResult{Kurzfazits: kurzfazits, Summary: strings.TrimSpace(wire.Summary)}, nil
}

const rejectionAnalysisSystem = `You analyze why a user rejected a real-estate listing, to see whether the rejection reveals a durable preference not yet captured in their evaluation criteria or profile notes.

You receive: the rejected listing's key facts, the user's rejection comment, the profile's current criteria (with IDs, name, description), and the profile's current context notes (freeform rules the profile already follows).

Decide whether the rejection comment reveals:
1. A new evaluation criterion that should exist, or an existing one (reference it by its given ID) that should be edited — e.g. "zu teuer für die Grundstücksgröße" might mean a price criterion should weigh land size, or a missing criterion entirely.
2. A durable rule for how to evaluate or source future listings — e.g. "Makler antwortet nie" might mean a context note about that seller/portal.

Most rejections need NO proposal at all — something like "schon verkauft" or a reason already well covered by existing criteria/context reveals nothing new. Only propose when the comment reveals something durable that isn't already covered. It is completely normal and expected to return two empty arrays.

Return ONLY a JSON object:
{"criteria_proposals": [{"action": "add"|"edit", "criterion_id": "<existing id, only for edit>", "name": "...", "description": "...", "is_subjective": true|false, "weight": <1-5, optional>}], "context_proposals": [{"scope": "profile", "operation": "append", "content": "..."}]}

No prose outside the JSON, no markdown fences.`

// rawRejectionAnalysis is the wire shape of AnalyzeRejection's response.
type rawRejectionAnalysis struct {
	CriteriaProposals []rawCriteriaProposal `json:"criteria_proposals"`
	ContextProposals  []rawContextProposal  `json:"context_proposals"`
}

type rawCriteriaProposal struct {
	Action       string `json:"action"`
	CriterionID  string `json:"criterion_id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	IsSubjective bool   `json:"is_subjective"`
	Weight       *int   `json:"weight"`
}

type rawContextProposal struct {
	Scope     string `json:"scope"`
	Operation string `json:"operation"`
	Content   string `json:"content"`
}

// runAnalyzeRejection implements Provider.AnalyzeRejection against any
// completer.
func runAnalyzeRejection(
	ctx context.Context,
	c completer,
	listing domain.Listing,
	rejectionComment string,
	criteria []domain.Criterion,
	contextText string,
) (RejectionAnalysisResult, error) {
	cfg := c.config()

	var user strings.Builder
	fmt.Fprintf(&user, "Reasoning language: %s.\n\n", cfg.reasoningLanguage())
	user.WriteString("Rejected listing:\n")
	user.WriteString(renderListing(listing))
	fmt.Fprintf(&user, "\n\nRejection comment: %s\n", strings.TrimSpace(rejectionComment))

	user.WriteString("\nCurrent criteria:\n")
	if len(criteria) == 0 {
		user.WriteString("(none)\n")
	}
	for _, criterion := range criteria {
		kind := "objective"
		if criterion.IsSubjective {
			kind = "subjective"
		}
		fmt.Fprintf(&user, "- id=%s [%s] %s — %s\n", criterion.ID, kind, criterion.Name, criterion.Description)
	}

	user.WriteString("\nCurrent context notes:\n")
	if strings.TrimSpace(contextText) == "" {
		user.WriteString("(none)\n")
	} else {
		user.WriteString(contextText)
		user.WriteString("\n")
	}

	return callWithRetries(ctx, c, rejectionAnalysisSystem, user.String(), func(raw string) (RejectionAnalysisResult, error) {
		return parseRejectionAnalysis(raw, criterionIDSet(criteria))
	})
}

// criterionIDSet builds the lookup parseRejectionAnalysis uses to validate
// an "edit" proposal's criterion_id references a real Criterion.
func criterionIDSet(criteria []domain.Criterion) map[string]bool {
	ids := make(map[string]bool, len(criteria))
	for _, criterion := range criteria {
		ids[criterion.ID.String()] = true
	}
	return ids
}

// parseRejectionAnalysis validates the model's proposals eagerly — an
// invalid action/scope/operation, or an "edit" referencing an unknown
// criterion id, is an ErrParse so the corrective retry fires, same as
// parseGrades/parseRanking's shape checks.
func parseRejectionAnalysis(raw string, criterionIDs map[string]bool) (RejectionAnalysisResult, error) {
	value, err := extractFirstJSON(raw)
	if err != nil {
		return RejectionAnalysisResult{}, err
	}
	var wire rawRejectionAnalysis
	if err := json.Unmarshal([]byte(value), &wire); err != nil {
		return RejectionAnalysisResult{}, fmt.Errorf("unmarshal rejection analysis: %w", ErrParse)
	}

	result := RejectionAnalysisResult{
		CriteriaProposals: make([]CriteriaProposal, 0, len(wire.CriteriaProposals)),
		ContextProposals:  make([]ContextProposal, 0, len(wire.ContextProposals)),
	}
	for _, proposal := range wire.CriteriaProposals {
		if proposal.Action != "add" && proposal.Action != "edit" {
			return RejectionAnalysisResult{}, fmt.Errorf("invalid criteria_proposals action %q: %w", proposal.Action, ErrParse)
		}
		if strings.TrimSpace(proposal.Name) == "" {
			return RejectionAnalysisResult{}, fmt.Errorf("empty criteria_proposals name: %w", ErrParse)
		}
		var criterionID *string
		if proposal.Action == "edit" {
			if !criterionIDs[proposal.CriterionID] {
				return RejectionAnalysisResult{}, fmt.Errorf("edit proposal references unknown criterion_id %q: %w", proposal.CriterionID, ErrParse)
			}
			id := proposal.CriterionID
			criterionID = &id
		}
		result.CriteriaProposals = append(result.CriteriaProposals, CriteriaProposal{
			Action:       proposal.Action,
			CriterionID:  criterionID,
			Name:         strings.TrimSpace(proposal.Name),
			Description:  strings.TrimSpace(proposal.Description),
			IsSubjective: proposal.IsSubjective,
			Weight:       proposal.Weight,
		})
	}
	for _, proposal := range wire.ContextProposals {
		if proposal.Scope != "global" && proposal.Scope != "profile" {
			return RejectionAnalysisResult{}, fmt.Errorf("invalid context_proposals scope %q: %w", proposal.Scope, ErrParse)
		}
		if proposal.Operation != "append" {
			return RejectionAnalysisResult{}, fmt.Errorf("invalid context_proposals operation %q: %w", proposal.Operation, ErrParse)
		}
		if strings.TrimSpace(proposal.Content) == "" {
			return RejectionAnalysisResult{}, fmt.Errorf("empty context_proposals content: %w", ErrParse)
		}
		result.ContextProposals = append(result.ContextProposals, ContextProposal{
			Scope:     proposal.Scope,
			Operation: proposal.Operation,
			Content:   strings.TrimSpace(proposal.Content),
		})
	}
	return result, nil
}

// renderListing serializes the evaluation-relevant listing fields as
// compact labeled lines, omitting empty optional fields.
func renderListing(listing domain.Listing) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", listing.Title)
	writeFloat(&b, "Price", listing.Price, listing.PriceCurrency)
	writeFloatUnit(&b, "Property size", listing.PropertySizeM2, "m²")
	writeFloatUnit(&b, "Plot size", listing.PlotSizeM2, "m²")
	writeString(&b, "Address", listing.Address)
	writeString(&b, "Region", listing.Region)
	writeString(&b, "Country", listing.Country)
	if listing.Latitude != nil && listing.Longitude != nil {
		fmt.Fprintf(&b, "Coordinates: %.6f, %.6f\n", *listing.Latitude, *listing.Longitude)
	}
	writeString(&b, "Description", listing.Description)
	return strings.TrimRight(b.String(), "\n")
}

func writeString(b *strings.Builder, label string, v *string) {
	if v != nil && strings.TrimSpace(*v) != "" {
		fmt.Fprintf(b, "%s: %s\n", label, strings.TrimSpace(*v))
	}
}

func writeFloat(b *strings.Builder, label string, v *float64, currency *string) {
	if v == nil {
		return
	}
	cur := ""
	if currency != nil {
		cur = " " + strings.TrimSpace(*currency)
	}
	fmt.Fprintf(b, "%s: %.2f%s\n", label, *v, cur)
}

func writeFloatUnit(b *strings.Builder, label string, v *float64, unit string) {
	if v == nil {
		return
	}
	fmt.Fprintf(b, "%s: %.2f %s\n", label, *v, unit)
}
