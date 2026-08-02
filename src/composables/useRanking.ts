import { readonly, shallowRef } from 'vue';

import { getProfile } from '@/api/profiles';
import { refreshRanking } from '@/api/ranking';

import type { Listing } from '@/types/listing';

/**
 * Owns search_profile.ranking_summary and the "Rangliste aktualisieren"
 * action (spec 01-05 Layer 5.2). onListingsUpdated lets the caller merge
 * the refreshed per-listing kurzfazit text back into its own listing state
 * without a second fetch.
 */
export function useRanking(profileId: string, onListingsUpdated: (listings: Listing[]) => void) {
  const summary = shallowRef<string | undefined>(undefined);
  const loadingSummary = shallowRef(false);
  const refreshing = shallowRef(false);
  const error = shallowRef<string | null>(null);

  async function fetchSummary() {
    loadingSummary.value = true;
    try {
      const profile = await getProfile(profileId);
      summary.value = profile.ranking_summary;
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    } finally {
      loadingSummary.value = false;
    }
  }

  async function refresh() {
    refreshing.value = true;
    error.value = null;
    try {
      const result = await refreshRanking(profileId);
      summary.value = result.profile.ranking_summary;
      onListingsUpdated(result.listings);
    } catch (refreshError) {
      error.value = refreshError instanceof Error ? refreshError.message : String(refreshError);
    } finally {
      refreshing.value = false;
    }
  }

  return {
    summary: readonly(summary),
    loadingSummary: readonly(loadingSummary),
    refreshing: readonly(refreshing),
    error: readonly(error),
    fetchSummary,
    refresh,
  };
}
