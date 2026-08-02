import { readonly, shallowRef } from 'vue';

import * as criteriaApi from '@/api/criteria';

import type { CriterionInput } from '@/api/criteria';
import type { Criterion } from '@/types/criterion';

/** Loads a profile's Criteria and exposes create/update/delete actions that
 * keep the local list in sync with the backend. */
export function useCriteria(profileId: string) {
  const criteria = shallowRef<Criterion[]>([]);
  const loading = shallowRef(false);
  const error = shallowRef<string | null>(null);

  async function fetchCriteria() {
    loading.value = true;
    error.value = null;
    try {
      criteria.value = await criteriaApi.listCriteria(profileId);
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    } finally {
      loading.value = false;
    }
  }

  async function createCriterion(input: CriterionInput) {
    const created = await criteriaApi.createCriterion(profileId, input);
    criteria.value = [...criteria.value, created];
    return created;
  }

  async function updateCriterion(criterionId: string, input: CriterionInput) {
    const updated = await criteriaApi.updateCriterion(criterionId, input);
    criteria.value = criteria.value.map((criterion) =>
      criterion.id === updated.id ? updated : criterion,
    );
    return updated;
  }

  async function deleteCriterion(criterionId: string) {
    await criteriaApi.deleteCriterion(criterionId);
    criteria.value = criteria.value.filter((criterion) => criterion.id !== criterionId);
  }

  return {
    criteria: readonly(criteria),
    loading: readonly(loading),
    error: readonly(error),
    fetchCriteria,
    createCriterion,
    updateCriterion,
    deleteCriterion,
  };
}
