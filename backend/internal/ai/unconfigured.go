package ai

import (
	"context"
	"propertyfinder/internal/domain"
)

// unconfiguredProvider is the Provider used when no LLM backend is
// configured (empty provider, missing API key, or a reserved-but-not-yet-
// implemented value). Every operation fails fast with ErrNotConfigured so
// LLM-dependent features surface "llm_not_configured" instead of crashing.
type unconfiguredProvider struct{}

func (unconfiguredProvider) EvaluateListing(context.Context, domain.Listing, []domain.Criterion) ([]domain.CriterionScore, error) {
	return nil, ErrNotConfigured
}
