package agent

import (
	"context"
	"propertyfinder/internal/domain"
)

// unconfiguredProvider is used when no LLM backend is configured
// (llm.api_key missing) — Chat fails fast with ErrNotConfigured instead of
// main.go crashing on a nil Provider (mirrors internal/ai's unconfigured
// provider).
type unconfiguredProvider struct{}

// NewUnconfiguredProvider constructs the stand-in Provider used when
// llm.api_key is empty.
func NewUnconfiguredProvider() Provider { return &unconfiguredProvider{} }

func (*unconfiguredProvider) SupportsDocumentInput() bool { return false }

func (*unconfiguredProvider) FormatTools([]Tool) any { return nil }

// FormatMessages still does the DB-row-to-Message mapping (text content
// only — no provider-specific tool_result merging since there's no
// provider to merge for) so callers can build a TurnRequest before
// discovering Chat will fail.
func (*unconfiguredProvider) FormatMessages(messages []domain.ChatMessage) []Message {
	result := make([]Message, 0, len(messages))
	for _, m := range messages {
		role := "user"
		if m.Role == domain.ChatRoleAssistant {
			role = "assistant"
		}
		result = append(result, Message{Role: role, Content: []ContentBlock{{Type: ContentBlockText, Text: m.Content}}})
	}
	return result
}

func (*unconfiguredProvider) Chat(context.Context, ChatRequest) (<-chan ChatEvent, error) {
	return nil, ErrNotConfigured
}
