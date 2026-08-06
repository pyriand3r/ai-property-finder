// Package ai holds the single-shot LLM operations kept from
// dev-property-finder (spec 00-03 "the single-shot operations
// (ParseSearchProfile, EvaluateListing) are kept — they remain the cleanest
// way to get one structured result"). This is distinct from the multi-turn
// Agent loop (internal/agent, Layer 6+) — Provider here is called directly
// by services, one request in, one structured result out.
package ai

import (
	"context"
	"propertyfinder/internal/domain"
)

// Provider abstracts the swappable LLM backend. Services depend on this
// interface only, never on a concrete client.
type Provider interface {
	// EvaluateListing returns the per-Criterion Grade and reasoning for one
	// listing against the given ordered criteria.
	EvaluateListing(ctx context.Context, listing domain.Listing, criteria []domain.Criterion) ([]domain.CriterionScore, error)

	// GenerateRanking writes a short Kurzfazit for each listing (same
	// order as the input, mapped back by position like EvaluateListing —
	// ADR 0006) plus one profile-wide comparison summary (spec 01-05:
	// "isolierter LLM-Call... kein Agent involviert"). listings must
	// already be sorted by Score desc and pre-filtered to those with a
	// Score (the caller's job, not the model's).
	GenerateRanking(ctx context.Context, listings []domain.Listing) (RankingResult, error)

	// AnalyzeRejection looks at why a Listing was rejected and decides
	// whether the comment reveals a durable preference not yet captured —
	// producing zero or more Criteria/Context proposals (spec 01-04
	// "Rejection Feedback, Reimagined"). Single-shot, no tools — same
	// shape as EvaluateListing/GenerateRanking. criteria lets the model
	// reference an existing Criterion by ID for an "edit" proposal;
	// contextText is the profile's current Context (spec 01-02), so the
	// model doesn't propose something already covered.
	AnalyzeRejection(ctx context.Context, listing domain.Listing, rejectionComment string, criteria []domain.Criterion, contextText string) (RejectionAnalysisResult, error)
}

// RankingResult is GenerateRanking's output. Kurzfazits has exactly
// len(listings) entries, in the same order as the input.
type RankingResult struct {
	Kurzfazits []string
	Summary    string
}

// RejectionAnalysisResult is AnalyzeRejection's output — the raw shape of
// zero or more propose_criteria / update_context proposals (agent/tools.go
// InputSchema), before the caller turns them into agent.StoredToolCall rows
// for a synthetic chat_message. Both slices are frequently empty — most
// rejections reveal nothing new.
type RejectionAnalysisResult struct {
	CriteriaProposals []CriteriaProposal
	ContextProposals  []ContextProposal
}

// CriteriaProposal mirrors propose_criteria's InputSchema.
type CriteriaProposal struct {
	Action       string // "add" | "edit"
	CriterionID  *string
	Name         string
	Description  string
	IsSubjective bool
	Weight       *int
}

// ContextProposal mirrors update_context's InputSchema.
type ContextProposal struct {
	Scope     string // "global" | "profile"
	Operation string // "append" — the only value the tool supports
	Content   string
}
