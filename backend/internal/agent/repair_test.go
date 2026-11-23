package agent

import (
	"encoding/json"
	"testing"
)

func textMsg(role, text string) Message {
	return Message{Role: role, Content: []ContentBlock{{Type: ContentBlockText, Text: text}}}
}

func toolUseMsg(ids ...string) Message {
	message := Message{Role: "assistant"}
	for _, id := range ids {
		message.Content = append(message.Content, ContentBlock{
			Type: ContentBlockToolUse, ToolUseID: id, ToolName: "fetch_url", ToolInput: json.RawMessage(`{}`),
		})
	}
	return message
}

func toolResultMsg(ids ...string) Message {
	message := Message{Role: "user"}
	for _, id := range ids {
		message.Content = append(message.Content, ContentBlock{
			Type: ContentBlockToolResult, ToolResultForID: id, ToolResultText: `{"ok":true}`,
		})
	}
	return message
}

// assertPaired fails unless messages obey Claude's pairing rule: the user
// message right after an assistant tool_use answers exactly its ids, with
// the tool_result blocks first, and no other message holds a tool_result.
func assertPaired(t *testing.T, messages []Message) {
	t.Helper()
	for i, message := range messages {
		ids := toolUseIDs(message)
		if len(ids) == 0 {
			if message.Role == "user" && (i == 0 || len(toolUseIDs(messages[i-1])) == 0) &&
				len(withoutToolResults(message.Content)) != len(message.Content) {
				t.Errorf("messages[%d]: orphan tool_result", i)
			}
			continue
		}
		if i+1 >= len(messages) || messages[i+1].Role != "user" {
			t.Errorf("messages[%d]: tool_use %v not followed by a user message", i, ids)
			continue
		}
		next := messages[i+1].Content
		if len(next) < len(ids) {
			t.Errorf("messages[%d]: tool_use %v, next message has only %d blocks", i, ids, len(next))
			continue
		}
		for j, id := range ids {
			if next[j].Type != ContentBlockToolResult || next[j].ToolResultForID != id {
				t.Errorf("messages[%d]: block %d of next message = %+v, want tool_result for %s", i, j, next[j], id)
			}
		}
		for _, block := range next[len(ids):] {
			if block.Type == ContentBlockToolResult {
				t.Errorf("messages[%d]: surplus tool_result %q", i, block.ToolResultForID)
			}
		}
	}
}

// TestRepairToolPairing_DanglingToolUseBeforeNextUserMessage is the
// buglist scenario: the server stopped after persisting the assistant
// tool_use row but before the role=tool rows, so the next user message
// follows the tool_use directly. Sent as is, Claude answers 400 on every
// later turn.
func TestRepairToolPairing_DanglingToolUseBeforeNextUserMessage(t *testing.T) {
	history := []Message{
		textMsg("user", "Schau dir das an"),
		toolUseMsg("toolu_1"),
		textMsg("user", "Und jetzt?"),
	}

	repaired := RepairToolPairing(history)

	assertPaired(t, repaired)
	if len(repaired) != 3 {
		t.Fatalf("got %d messages, want 3 (results merged into the user message, not a new one): %+v", len(repaired), repaired)
	}
	first := repaired[2].Content[0]
	if first.Type != ContentBlockToolResult || first.ToolResultForID != "toolu_1" || !first.ToolResultError || first.ToolResultText != InterruptedToolResult {
		t.Errorf("first block = %+v, want synthetic error tool_result for toolu_1", first)
	}
	if last := repaired[2].Content[len(repaired[2].Content)-1]; last.Type != ContentBlockText || last.Text != "Und jetzt?" {
		t.Errorf("user text lost or reordered: %+v", repaired[2].Content)
	}
}

func TestRepairToolPairing_DanglingToolUseAtEndGetsOwnUserMessage(t *testing.T) {
	repaired := RepairToolPairing([]Message{textMsg("user", "hi"), toolUseMsg("toolu_1", "toolu_2")})

	assertPaired(t, repaired)
	if len(repaired) != 3 || repaired[2].Role != "user" || len(repaired[2].Content) != 2 {
		t.Errorf("repaired = %+v, want a trailing user message with two synthetic results", repaired)
	}
}

func TestRepairToolPairing_DanglingToolUseFollowedByAssistant(t *testing.T) {
	repaired := RepairToolPairing([]Message{
		textMsg("user", "hi"), toolUseMsg("toolu_1"), textMsg("assistant", "fertig"), textMsg("user", "weiter"),
	})

	assertPaired(t, repaired)
	if len(repaired) != 5 {
		t.Errorf("got %d messages, want 5 (synthetic user message inserted after the tool_use): %+v", len(repaired), repaired)
	}
}

func TestRepairToolPairing_PartialResultsKeepRealOnesAndFillGap(t *testing.T) {
	repaired := RepairToolPairing([]Message{
		textMsg("user", "hi"), toolUseMsg("toolu_1", "toolu_2"), toolResultMsg("toolu_2"), textMsg("assistant", "ok"),
	})

	assertPaired(t, repaired)
	blocks := repaired[2].Content
	if len(blocks) != 2 || blocks[0].ToolResultForID != "toolu_1" || !blocks[0].ToolResultError {
		t.Fatalf("blocks = %+v, want synthetic toolu_1 first", blocks)
	}
	if blocks[1].ToolResultForID != "toolu_2" || blocks[1].ToolResultError || blocks[1].ToolResultText != `{"ok":true}` {
		t.Errorf("real toolu_2 result was altered: %+v", blocks[1])
	}
}

// An interleaved write (two turns on one session) can leave a late
// tool_result behind a user message; it answers no tool_use of the message
// before it and Claude rejects it too.
func TestRepairToolPairing_DropsOrphanToolResults(t *testing.T) {
	repaired := RepairToolPairing([]Message{
		textMsg("user", "hi"),
		toolUseMsg("toolu_1"),
		textMsg("user", "zwischendrin"),
		toolResultMsg("toolu_1"),
		textMsg("assistant", "ok"),
	})

	assertPaired(t, repaired)
	for i, message := range repaired {
		if message.Role == "user" && len(message.Content) == 0 {
			t.Errorf("messages[%d] is an empty user message", i)
		}
	}
}

func TestRepairToolPairing_ValidHistoryUnchangedAndInputNotMutated(t *testing.T) {
	history := []Message{
		textMsg("user", "hi"),
		toolUseMsg("toolu_1", "toolu_2"),
		toolResultMsg("toolu_1", "toolu_2"),
		textMsg("assistant", "fertig"),
		textMsg("user", "danke"),
	}

	repaired := RepairToolPairing(history)

	if len(repaired) != len(history) {
		t.Fatalf("got %d messages, want %d", len(repaired), len(history))
	}
	for i := range history {
		if len(repaired[i].Content) != len(history[i].Content) || repaired[i].Role != history[i].Role {
			t.Errorf("messages[%d] changed: got %+v, want %+v", i, repaired[i], history[i])
		}
	}
	for _, block := range repaired[2].Content {
		if block.ToolResultError {
			t.Errorf("real tool_result %q was replaced by a synthetic one", block.ToolResultForID)
		}
	}
}

func TestRepairToolPairing_DoesNotMutateInput(t *testing.T) {
	history := []Message{textMsg("user", "hi"), toolUseMsg("toolu_1"), textMsg("user", "weiter")}

	RepairToolPairing(history)

	if len(history[2].Content) != 1 {
		t.Errorf("input user message was modified: %+v", history[2].Content)
	}
}
