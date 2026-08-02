package ai

import (
	"context"
	"errors"
	"propertyfinder/internal/domain"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// claudeMaxTokens caps the JSON response. Generous enough for a few dozen
// criteria with per-criterion German reasoning; JSON output never needs a
// large ceiling.
const claudeMaxTokens = 8192

// claudeProvider talks to Anthropic's Messages API. The prompt design, JSON
// robustness, and retry logic live in ops.go/retry.go; this type is only
// the transport.
type claudeProvider struct {
	cfg    Config
	client anthropic.Client
}

func newClaudeProvider(cfg Config) *claudeProvider {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &claudeProvider{cfg: cfg, client: anthropic.NewClient(opts...)}
}

func (p *claudeProvider) config() Config {
	return p.cfg
}

// complete sends system+user and returns the raw assistant text. It bounds
// the call with the per-call timeout and maps a deadline or a provider
// context-length error onto the ErrTimeout / ErrListingTooLong sentinels.
func (p *claudeProvider) complete(ctx context.Context, system, user string) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, p.cfg.timeout())
	defer cancel()

	resp, err := p.client.Messages.New(callCtx, anthropic.MessageNewParams{
		Model:     p.cfg.Model,
		MaxTokens: claudeMaxTokens,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(user)),
		},
	})
	if err != nil {
		return "", classifyProviderError(callCtx, err)
	}

	var text strings.Builder
	for _, block := range resp.Content {
		if textBlock, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(textBlock.Text)
		}
	}
	return text.String(), nil
}

// classifyProviderError maps a transport error onto the ai sentinels. A
// context deadline becomes ErrTimeout; a context-length error becomes
// ErrListingTooLong; everything else is returned as-is (retryable).
func classifyProviderError(callCtx context.Context, err error) error {
	if errors.Is(callCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		if isContextLengthError(apiErr.Error()) {
			return ErrListingTooLong
		}
	} else if isContextLengthError(err.Error()) {
		return ErrListingTooLong
	}
	return err
}

func (p *claudeProvider) EvaluateListing(ctx context.Context, listing domain.Listing, criteria []domain.Criterion) ([]domain.CriterionScore, error) {
	return runEvaluate(ctx, p, listing, criteria)
}

func (p *claudeProvider) GenerateRanking(ctx context.Context, listings []domain.Listing) (RankingResult, error) {
	return runGenerateRanking(ctx, p, listings)
}
