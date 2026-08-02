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

const evaluateSystem = `You grade a real-estate listing against an ordered, numbered list of criteria.

For EACH criterion, assign a letter grade A (best) to F (worst) and a short reasoning text. Grade only against the criterion's meaning; if the listing lacks the information a criterion needs, grade it low and say so.

Return ONLY a JSON array with EXACTLY one object per criterion, in the SAME ORDER as the numbered list:
{"index": <1-based position>, "grade": "A".."F", "reasoning": "<short explanation>"}

The array length MUST equal the number of criteria. No prose, no markdown fences.`

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

	return callWithRetries(ctx, c, evaluateSystem, user.String(), func(raw string) ([]domain.CriterionScore, error) {
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
		grade, gradeErr := normalizeGrade(w.Grade)
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
