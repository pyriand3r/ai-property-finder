import { http } from '@/api/http';

import type { Criterion } from '@/types/criterion';

export function listCriteria(profileId: string) {
  return http.get<Criterion[]>(`/api/profiles/${profileId}/criteria`);
}

export interface CriterionInput {
  name?: string;
  description?: string;
  weight?: number;
  is_subjective?: boolean;
}

export function createCriterion(profileId: string, input: CriterionInput) {
  return http.post<Criterion>(`/api/profiles/${profileId}/criteria`, input);
}

export function updateCriterion(criterionId: string, input: CriterionInput) {
  return http.patch<Criterion>(`/api/criteria/${criterionId}`, input);
}

/** Turns a proposed criterion into an active one (docs/adr/0024). */
export function approveCriterion(criterionId: string) {
  return http.post<Criterion>(`/api/criteria/${criterionId}/approve`);
}

export function deleteCriterion(criterionId: string) {
  return http.delete<void>(`/api/criteria/${criterionId}`);
}
