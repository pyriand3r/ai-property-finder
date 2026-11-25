package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"propertyfinder/internal/domain"
	"strings"
)

// openaiMaxTokens caps the JSON response, same ceiling as claudeMaxTokens.
const openaiMaxTokens = 8192

// maxErrorBody bounds how much of a non-2xx response goes into the error.
const maxErrorBody = 2048

// openaiProvider is the transport for OpenAI-compatible Chat Completions
// endpoints (LM Studio, OpenCode; ADR 0030). Prompt design, JSON robustness
// and retries live in ops.go/retry.go, shared with claudeProvider.
type openaiProvider struct {
	cfg    Config
	client *http.Client
}

func newOpenAIProvider(cfg Config) *openaiProvider {
	return &openaiProvider{cfg: cfg, client: &http.Client{}}
}

func (p *openaiProvider) config() Config { return p.cfg }

type openaiRequest struct {
	Model     string          `json:"model"`
	Messages  []openaiMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// complete sends system+user as one non-streaming request and returns the
// assistant text, mapping a deadline and a context-overflow error onto the
// ErrTimeout / ErrListingTooLong sentinels (see classifyProviderError).
func (p *openaiProvider) complete(ctx context.Context, system, user string) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, p.cfg.timeout())
	defer cancel()

	body, err := json.Marshal(openaiRequest{
		Model:     p.cfg.Model,
		Messages:  []openaiMessage{{Role: "system", Content: system}, {Role: "user", Content: user}},
		MaxTokens: openaiMaxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("encode chat request: %w", err)
	}
	endpoint := strings.TrimRight(strings.TrimSpace(p.cfg.BaseURL), "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(p.cfg.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", classifyProviderError(callCtx, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return "", classifyProviderError(callCtx, fmt.Errorf("llm endpoint returned %s: %s", resp.Status, strings.TrimSpace(string(snippet))))
	}
	var decoded openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", classifyProviderError(callCtx, fmt.Errorf("decode chat response: %w", err))
	}
	if len(decoded.Choices) == 0 {
		return "", errors.New("llm endpoint returned no choices")
	}
	return decoded.Choices[0].Message.Content, nil
}

func (p *openaiProvider) EvaluateListing(ctx context.Context, listing domain.Listing, criteria []domain.Criterion) ([]domain.CriterionScore, error) {
	return runEvaluate(ctx, p, listing, criteria)
}

func (p *openaiProvider) GenerateRanking(ctx context.Context, listings []domain.Listing) (RankingResult, error) {
	return runGenerateRanking(ctx, p, listings)
}

func (p *openaiProvider) AnalyzeRejection(ctx context.Context, listing domain.Listing, rejectionComment string, criteria []domain.Criterion, contextText string) (RejectionAnalysisResult, error) {
	return runAnalyzeRejection(ctx, p, listing, rejectionComment, criteria, contextText)
}
