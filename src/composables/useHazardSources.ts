import { readonly, shallowRef } from 'vue';

import * as hazardsApi from '@/api/hazards';

import type { HazardCatalogEntry, HazardSourceSuggestion } from '@/types/hazard';

/**
 * Loads the hazard source catalogue and the Agent's source suggestions, and lets the user approve
 * or reject a suggestion (ADR 0027, spec 01-15). Approving only clears a source for a later
 * release; nothing is ingested at runtime (ADR 0025).
 */
export function useHazardSources() {
  const catalog = shallowRef<HazardCatalogEntry[]>([]);
  const suggestions = shallowRef<HazardSourceSuggestion[]>([]);
  const loading = shallowRef(false);
  const error = shallowRef<string | null>(null);

  async function fetchAll() {
    loading.value = true;
    error.value = null;
    try {
      [catalog.value, suggestions.value] = await Promise.all([
        hazardsApi.listHazardSources(),
        hazardsApi.listSourceSuggestions(),
      ]);
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    } finally {
      loading.value = false;
    }
  }

  function replace(updated: HazardSourceSuggestion) {
    suggestions.value = suggestions.value.map((suggestion) =>
      suggestion.id === updated.id ? updated : suggestion,
    );
    return updated;
  }

  async function approve(suggestionId: string) {
    return replace(await hazardsApi.approveSourceSuggestion(suggestionId));
  }

  async function reject(suggestionId: string) {
    return replace(await hazardsApi.rejectSourceSuggestion(suggestionId));
  }

  return {
    catalog: readonly(catalog),
    suggestions: readonly(suggestions),
    loading: readonly(loading),
    error: readonly(error),
    fetchAll,
    approve,
    reject,
  };
}
