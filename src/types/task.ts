/** Mirrors backend/internal/domain.Task JSON (spec 01-04 Layer 2.3). */
export interface Task {
  id: string;
  type: string;
  status: 'running' | 'succeeded' | 'failed';
  profile_id?: string;
  listing_id?: string;
  total?: number;
  done: number;
  failed_count: number;
  error?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  updated_at: string;
}

/** Payload shape published on the SSE broker for task.started/progress/finished
 * (backend/internal/service/evaluation_service.go taskEventPayload). */
export interface TaskEventPayload {
  task_id: string;
  type?: string;
  status?: Task['status'];
  total?: number | null;
  done: number;
  failed_count: number;
}
