/** Mirrors backend/internal/domain.Context JSON exactly (snake_case). */
export interface ContextFile {
  id: string;
  scope: 'global' | 'profile';
  profile_id?: string;
  content_md: string;
  created_at: string;
  updated_at: string;
}

/** Mirrors backend/internal/domain.ContextRevision JSON. content_md is the
 * snapshot from BEFORE the change that produced this row, not a diff. */
export interface ContextRevision {
  id: string;
  context_id: string;
  content_md: string;
  changed_by: 'user' | 'agent';
  chat_message_id?: string;
  created_at: string;
}
