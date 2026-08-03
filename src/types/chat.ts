/** Mirrors backend/internal/domain.ChatSession JSON (spec 01-01, ADR 0014). */
export interface ChatSession {
  id: string;
  profile_id: string;
  created_at: string;
  updated_at: string;
  ended_at?: string;
  token_count_estimate?: number;
}

/** Mirrors backend/internal/domain.ChatMessage JSON. tool_calls/tool_result
 * stay untyped (JSONB, Agent-only from Layer 6.4+) — the chat UI never
 * needs to read them directly, only display content/listing_id. */
export type ChatRole = 'user' | 'assistant' | 'tool';

export interface ChatMessage {
  id: string;
  session_id: string;
  role: ChatRole;
  content: string;
  tool_calls?: unknown;
  tool_result?: unknown;
  tool_name?: string;
  attachment_path?: string;
  listing_id?: string;
  created_at: string;
}
