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
}
