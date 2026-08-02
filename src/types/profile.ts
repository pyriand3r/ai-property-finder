/**
 * Mirrors backend/internal/domain.SearchProfile JSON exactly (snake_case
 * wire format, see AGENTS.md "Naming Convention"). Local variables built
 * from this data stay camelCase.
 */
export interface Profile {
  id: string;
  name: string;
  description: string;
  archived_at?: string;
  ranking_summary?: string;
  created_at: string;
  updated_at: string;
}
