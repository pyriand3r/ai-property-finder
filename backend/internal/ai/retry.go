package ai

import (
	"context"
	"errors"
	"strings"
)

// completer is the transport half of a provider: it sends a system+user
// prompt requesting JSON output and returns the raw completion text. It
// applies the per-call timeout and maps a transport/context timeout onto
// ErrTimeout, but does NOT retry — callWithRetries owns the retry budget.
type completer interface {
	complete(ctx context.Context, system, user string) (string, error)
	config() Config
}

// correctivePrompt is prepended to the user message on a retry after a
// parse/shape failure (ADR 0006).
const correctivePrompt = "Return ONLY valid JSON matching the schema — no prose, no markdown fences.\n\n"

// callWithRetries runs parse against the provider up to cfg.MaxRetries+1
// times. On the first attempt corrective is empty; every retry prepends
// correctivePrompt so the model is told to fix its shape. Terminal errors
// (not configured, timeout, listing too long, deadline) short-circuit
// immediately. A parse/shape failure surviving all attempts is returned as
// ErrParse.
func callWithRetries[T any](
	ctx context.Context,
	c completer,
	system, user string,
	parse func(raw string) (T, error),
) (T, error) {
	var zero T
	cfg := c.config()

	attempts := max(cfg.MaxRetries+1, 1)

	var lastErr error
	for attempt := range attempts {
		attemptUser := user
		if attempt > 0 {
			attemptUser = correctivePrompt + user
		}

		raw, err := c.complete(ctx, system, attemptUser)
		if err != nil {
			if isTerminal(err) {
				return zero, err
			}
			lastErr = err
			continue
		}

		result, parseErr := parse(raw)
		if parseErr == nil {
			return result, nil
		}
		lastErr = parseErr
	}

	if lastErr != nil && !errors.Is(lastErr, ErrParse) {
		return zero, errors.Join(ErrParse, lastErr)
	}
	if lastErr == nil {
		lastErr = ErrParse
	}
	return zero, lastErr
}

// isTerminal reports whether err should abort the retry loop immediately.
func isTerminal(err error) bool {
	return errors.Is(err, ErrNotConfigured) ||
		errors.Is(err, ErrTimeout) ||
		errors.Is(err, ErrListingTooLong) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled)
}

// isContextLengthError reports whether a raw provider error message looks
// like a context-window overflow, so it can be surfaced as
// ErrListingTooLong rather than a raw provider error.
func isContextLengthError(msg string) bool {
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "context length") ||
		strings.Contains(msg, "context window") ||
		strings.Contains(msg, "context_length") ||
		strings.Contains(msg, "maximum context") ||
		strings.Contains(msg, "too many tokens") ||
		strings.Contains(msg, "prompt is too long")
}
