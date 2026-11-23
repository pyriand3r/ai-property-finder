package agent

// InterruptedToolResult is the tool_result text RepairToolPairing injects
// for a tool_use that has no persisted result. It is a normal error result
// (ToolResultError=true) so the model sees the call did not complete and
// decides itself whether to repeat it.
const InterruptedToolResult = `{"error":"Tool-Aufruf wurde unterbrochen, es liegt kein Ergebnis vor."}`

// RepairToolPairing makes a replayed history satisfy the tool-pairing rule
// Claude enforces with a 400: every tool_use block in an assistant message
// must be answered by a tool_result block in the immediately following user
// message, and every tool_result must answer a tool_use of the message
// right before it. Any violation makes the API reject the whole request, so
// a single bad row would otherwise break every later turn of the session
// (docs/buglist.md "dangling tool_use", spec 01-09).
//
// Persisted history can violate the rule even though the loop never builds
// such a request in memory: the server stops between writing the assistant
// row and its role=tool rows (ADR 0008, no recovery), a tool row's INSERT
// fails and is only logged, two turns write into one session at once, or
// the row predates RF.1. Instead of trusting storage, this runs on every
// load (see AgentService.runTurn):
//
//   - a tool_use without a result gets a synthetic error result
//     (InterruptedToolResult), merged into the next user message if there
//     is one, else added as a new user message;
//   - a tool_result that answers no tool_use of the previous assistant
//     message is dropped, and a user message left empty by that is dropped.
//
// The input is not modified. Histories that already satisfy the rule come
// back unchanged in content. Provider-independent on purpose (ADR 0007):
// it works on the normalized Message type.
func RepairToolPairing(messages []Message) []Message {
	repaired := make([]Message, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		message := messages[i]

		if message.Role == "assistant" {
			repaired = append(repaired, message)
			ids := toolUseIDs(message)
			if len(ids) == 0 {
				continue
			}
			var following *Message
			if i+1 < len(messages) && messages[i+1].Role == "user" {
				following = &messages[i+1]
				i++
			}
			repaired = append(repaired, pairToolResults(ids, following))
			continue
		}

		// A user message not directly after an assistant tool_use: any
		// tool_result in it is an orphan.
		kept := withoutToolResults(message.Content)
		if len(kept) == 0 {
			continue
		}
		repaired = append(repaired, Message{Role: message.Role, Content: kept})
	}
	return repaired
}

func toolUseIDs(message Message) []string {
	var ids []string
	for _, block := range message.Content {
		if block.Type == ContentBlockToolUse {
			ids = append(ids, block.ToolUseID)
		}
	}
	return ids
}

func withoutToolResults(blocks []ContentBlock) []ContentBlock {
	kept := make([]ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Type != ContentBlockToolResult {
			kept = append(kept, block)
		}
	}
	return kept
}

// pairToolResults builds the user message answering ids: exactly one
// tool_result per id, in id order, taken from following when present and
// synthesized otherwise, followed by following's remaining (non-result)
// blocks. Claude requires tool_result blocks to come first in the message.
func pairToolResults(ids []string, following *Message) Message {
	found := make(map[string]ContentBlock, len(ids))
	var rest []ContentBlock
	if following != nil {
		for _, block := range following.Content {
			if block.Type != ContentBlockToolResult {
				rest = append(rest, block)
				continue
			}
			if _, dup := found[block.ToolResultForID]; !dup {
				found[block.ToolResultForID] = block
			}
		}
	}

	content := make([]ContentBlock, 0, len(ids)+len(rest))
	for _, id := range ids {
		if block, ok := found[id]; ok {
			content = append(content, block)
			continue
		}
		content = append(content, ContentBlock{
			Type:            ContentBlockToolResult,
			ToolResultForID: id,
			ToolResultText:  InterruptedToolResult,
			ToolResultError: true,
		})
	}
	return Message{Role: "user", Content: append(content, rest...)}
}
