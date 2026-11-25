package agent

import (
	"encoding/json"
	"propertyfinder/internal/domain"
	"strings"
)

// StoredHistoryMessages converts DB-neutral chat_message rows (ADR 0010)
// into normalized Messages. It is the shared body of every Provider's
// FormatMessages: tool results belong on a "user" turn as ToolResult content
// blocks, immediately following the assistant turn that requested them, and
// consecutive role=tool rows collapse into one user Message (all results of
// one assistant turn arrive together). A provider whose wire format wants
// something else (OpenAI: one role="tool" message per result) maps that in
// its own Chat, not here.
func StoredHistoryMessages(messages []domain.ChatMessage) []Message {
	result := make([]Message, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case domain.ChatRoleUser:
			result = append(result, Message{
				Role:    "user",
				Content: []ContentBlock{{Type: ContentBlockText, Text: m.Content}},
			})
		case domain.ChatRoleAssistant:
			result = append(result, Message{Role: "assistant", Content: storedAssistantBlocks(m)})
		case domain.ChatRoleTool:
			block := storedToolResultBlock(m)
			if n := len(result); n > 0 && result[n-1].Role == "user" && isToolResultMessage(result[n-1]) {
				result[n-1].Content = append(result[n-1].Content, block)
			} else {
				result = append(result, Message{Role: "user", Content: []ContentBlock{block}})
			}
		}
	}
	return result
}

func storedAssistantBlocks(m domain.ChatMessage) []ContentBlock {
	var blocks []ContentBlock
	if strings.TrimSpace(m.Content) != "" {
		blocks = append(blocks, ContentBlock{Type: ContentBlockText, Text: m.Content})
	}
	if len(m.ToolCalls) > 0 {
		var calls []StoredToolCall
		if err := json.Unmarshal(m.ToolCalls, &calls); err == nil {
			for _, call := range calls {
				blocks = append(blocks, ContentBlock{
					Type:      ContentBlockToolUse,
					ToolUseID: call.ID,
					ToolName:  call.Name,
					ToolInput: call.Input,
				})
			}
		}
	}
	return blocks
}

func storedToolResultBlock(m domain.ChatMessage) ContentBlock {
	var stored StoredToolResult
	_ = json.Unmarshal(m.ToolResult, &stored)
	return ContentBlock{
		Type:            ContentBlockToolResult,
		ToolResultForID: stored.ToolCallID,
		ToolResultText:  string(stored.Result),
	}
}

func isToolResultMessage(m Message) bool {
	for _, block := range m.Content {
		if block.Type != ContentBlockToolResult {
			return false
		}
	}
	return len(m.Content) > 0
}
