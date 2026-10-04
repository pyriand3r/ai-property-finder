package service

import (
	"encoding/json"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/domain"
	"testing"
)

// TestToolCallsJSON_MergesDispatchedCallsMissingFromBlocks guards against a
// bug observed live in Layer 6.6 testing: an assistant roundtrip that
// dispatched a tool (a real role=tool row with a ToolResultForID exists)
// but whose Content blocks didn't include the matching tool_use — leaving
// chat_message.tool_calls empty. Replayed on a later turn, the orphaned
// tool_result (no matching tool_use in the previous message) is rejected
// by Claude's API outright.
func TestToolCallsJSON_MergesDispatchedCallsMissingFromBlocks(t *testing.T) {
	results := []agent.ToolOutcome{
		{Call: agent.ToolCall{ID: "toolu_1", Name: "compute_ranking", Arguments: json.RawMessage(`{}`)}},
	}

	data := toolCallsJSON(nil, results)
	if data == nil {
		t.Fatal("expected non-nil tool_calls JSON when a result was dispatched")
	}

	var calls []agent.StoredToolCall
	if err := json.Unmarshal(data, &calls); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if len(calls) != 1 || calls[0].ID != "toolu_1" || calls[0].Name != "compute_ranking" {
		t.Errorf("calls = %+v, want one StoredToolCall for toolu_1/compute_ranking", calls)
	}
}

func TestToolCallsJSON_DoesNotDuplicateWhenBlockAndResultAgree(t *testing.T) {
	blocks := []agent.ContentBlock{
		{Type: agent.ContentBlockToolUse, ToolUseID: "toolu_1", ToolName: "fetch_url", ToolInput: json.RawMessage(`{"url":"https://x"}`)},
	}
	results := []agent.ToolOutcome{
		{Call: agent.ToolCall{ID: "toolu_1", Name: "fetch_url", Arguments: json.RawMessage(`{"url":"https://x"}`)}},
	}

	data := toolCallsJSON(blocks, results)
	var calls []agent.StoredToolCall
	if err := json.Unmarshal(data, &calls); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if len(calls) != 1 {
		t.Errorf("got %d calls, want exactly 1 (no duplicate from the defensive merge)", len(calls))
	}
}

func TestToolCallsJSON_NilWhenNoToolCallsAndNoResults(t *testing.T) {
	if data := toolCallsJSON(nil, nil); data != nil {
		t.Errorf("toolCallsJSON(nil, nil) = %s, want nil", data)
	}
}

func TestAttachedImagePath_JPEGSuffix(t *testing.T) {
	path := "uploads/image/test.jpg"
	history := []domain.ChatMessage{{Role: domain.ChatRoleUser, AttachmentPath: &path}}

	got, mediaType := attachedImagePath(history)

	if got == nil || *got != path {
		t.Fatalf("attachedImagePath() path = %v, want %q", got, path)
	}
	if mediaType != "image/jpeg" {
		t.Errorf("mediaType = %q, want image/jpeg", mediaType)
	}
}

func TestAttachedImagePath_PNGSuffix(t *testing.T) {
	path := "uploads/image/test.png"
	history := []domain.ChatMessage{{Role: domain.ChatRoleUser, AttachmentPath: &path}}

	got, mediaType := attachedImagePath(history)

	if got == nil || *got != path {
		t.Fatalf("attachedImagePath() path = %v, want %q", got, path)
	}
	if mediaType != "image/png" {
		t.Errorf("mediaType = %q, want image/png", mediaType)
	}
}

func TestAttachedImagePath_IgnoresPDFSuffix(t *testing.T) {
	path := "uploads/pdf/test.pdf"
	history := []domain.ChatMessage{{Role: domain.ChatRoleUser, AttachmentPath: &path}}

	if got, _ := attachedImagePath(history); got != nil {
		t.Errorf("attachedImagePath() = %v, want nil for a .pdf attachment", got)
	}
}

func TestAttachedImagePath_EmptyHistory(t *testing.T) {
	if got, _ := attachedImagePath(nil); got != nil {
		t.Errorf("attachedImagePath(nil) = %v, want nil", got)
	}
}
