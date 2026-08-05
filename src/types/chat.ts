/** Mirrors backend/internal/domain.ChatSession JSON (spec 01-01, ADR 0014). */
export interface ChatSession {
  id: string;
  profile_id: string;
  created_at: string;
  updated_at: string;
  ended_at?: string;
  token_count_estimate?: number;
}

/** Mirrors backend/internal/agent.StoredToolCall JSON — one element of
 * chat_message.tool_calls (spec 00-02, ADR 0010). input's shape depends on
 * name; see ProposeCriteriaArgs/UpdateContextArgs for the two tools the
 * chat UI renders a Confirm-Card for (Layer 6.5, ADR 0009). Every other
 * tool's input is display-irrelevant — only its chat.tool_progress event
 * and the resulting role=tool message/Listing card matter to the UI. */
export interface StoredToolCall {
  id: string;
  name: string;
  input: unknown;
  /** Set only once the user has actioned this call's Confirm-Card —
   * absent/undefined means still pending (ADR 0017). Only meaningful for
   * propose_criteria/update_context; every other tool never has this. */
  status?: 'confirmed' | 'rejected';
}

/** propose_criteria tool-call arguments (backend/internal/agent/tools.go). */
export interface ProposeCriteriaArgs {
  action: 'add' | 'edit';
  criterion_id?: string;
  name: string;
  description?: string;
  is_subjective?: boolean;
  weight?: number;
}

/** update_context tool-call arguments (backend/internal/agent/tools.go). */
export interface UpdateContextArgs {
  scope: 'global' | 'profile';
  operation: 'append';
  content: string;
}

/** Mirrors backend/internal/domain.ChatMessage JSON. tool_result stays
 * untyped — the chat UI never reads it directly, only content/tool_name
 * for a role=tool bubble's display text. */
export type ChatRole = 'user' | 'assistant' | 'tool';

export interface ChatMessage {
  id: string;
  session_id: string;
  role: ChatRole;
  content: string;
  tool_calls?: StoredToolCall[];
  tool_result?: unknown;
  tool_name?: string;
  attachment_path?: string;
  listing_id?: string;
  created_at: string;
}
