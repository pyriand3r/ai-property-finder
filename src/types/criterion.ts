/** Mirrors backend/internal/domain.Criterion JSON exactly (snake_case). */
export interface Criterion {
  id: string;
  profile_id: string;
  name: string;
  description: string;
  weight: number;
  is_subjective: boolean;
  sort_order: number;
  created_at: string;
  updated_at: string;
}
