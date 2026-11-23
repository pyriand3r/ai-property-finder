/** "proposed" criteria are ignored by evaluation and scoring until the user
 * approves them (docs/adr/0024). */
export type CriterionStatus = 'proposed' | 'active';

/** Mirrors backend/internal/domain.Criterion JSON exactly (snake_case). */
export interface Criterion {
  id: string;
  profile_id: string;
  name: string;
  description: string;
  weight: number;
  is_subjective: boolean;
  sort_order: number;
  status: CriterionStatus;
  created_at: string;
  updated_at: string;
}
