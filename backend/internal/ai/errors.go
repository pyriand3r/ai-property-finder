package ai

import "errors"

// Sentinel errors a Provider returns so callers can classify failures
// without depending on a concrete backend.
var (
	// ErrNotConfigured means no LLM backend is configured (llm.provider is
	// empty, or llm.api_key is missing for the claude provider). Not
	// retryable.
	ErrNotConfigured = errors.New("llm_not_configured")

	// ErrTimeout means the LLM call exceeded llm.timeout_seconds. Not
	// retryable — a retry would just time out again.
	ErrTimeout = errors.New("llm_timeout")

	// ErrParse means the LLM returned an unparseable or wrong-shaped
	// response even after the corrective retries were exhausted.
	ErrParse = errors.New("parse_error")

	// ErrListingTooLong means the provider rejected the prompt for
	// exceeding its context window. Not retryable — the input must shrink.
	ErrListingTooLong = errors.New("listing_too_long")
)
