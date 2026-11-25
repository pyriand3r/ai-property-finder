/** Mirrors backend/internal/domain.SavedSearchStatus (ADR 0029). */
export type SavedSearchStatus = 'idle' | 'running' | 'succeeded' | 'failed';

/** Mirrors backend/internal/domain.SavedSearch JSON (spec 01-17). */
export interface SavedSearch {
  id: string;
  profile_id: string;
  name: string;
  url: string;
  page_param: string;
  max_pages: number;
  link_contains: string;
  interval_hours: number;
  enabled: boolean;
  /** Absent while the search is disabled. */
  next_run_at?: string;
  import_new: boolean;
  max_import: number;
  last_status: SavedSearchStatus;
  last_run_at?: string;
  last_task_id?: string;
  last_found: number;
  last_new: number;
  last_imported: number;
  last_summary: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
}
