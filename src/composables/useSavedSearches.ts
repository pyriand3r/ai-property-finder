import { onScopeDispose, readonly, shallowRef } from 'vue';

import * as savedSearchesApi from '@/api/savedSearches';

import type { SavedSearchInput } from '@/api/savedSearches';
import type { SavedSearch } from '@/types/savedSearch';

/** How often a list that holds a running search is reloaded to show its outcome. */
const POLL_MS = 3000;

/**
 * Loads a profile's saved portal searches (spec 01-17, ADR 0029) and exposes create/update/
 * delete/run actions that keep the list in sync. A run is started in the background (202), so
 * while any search is `running` the list reloads every few seconds until none is.
 */
export function useSavedSearches(profileId: string) {
  const searches = shallowRef<SavedSearch[]>([]);
  const loading = shallowRef(false);
  const error = shallowRef<string | null>(null);
  let pollTimer: ReturnType<typeof setTimeout> | undefined;

  function schedulePoll() {
    clearTimeout(pollTimer);
    if (searches.value.some((search) => search.last_status === 'running')) {
      pollTimer = setTimeout(() => void fetchSearches(false), POLL_MS);
    }
  }

  async function fetchSearches(showLoading = true) {
    if (showLoading) loading.value = true;
    error.value = null;
    try {
      searches.value = await savedSearchesApi.listSavedSearches(profileId);
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    } finally {
      loading.value = false;
      schedulePoll();
    }
  }

  function replace(updated: SavedSearch) {
    searches.value = searches.value.map((search) => (search.id === updated.id ? updated : search));
    schedulePoll();
    return updated;
  }

  async function createSearch(input: SavedSearchInput) {
    const created = await savedSearchesApi.createSavedSearch(profileId, input);
    searches.value = [...searches.value, created];
    return created;
  }

  async function updateSearch(searchId: string, input: SavedSearchInput) {
    return replace(await savedSearchesApi.updateSavedSearch(profileId, searchId, input));
  }

  async function deleteSearch(searchId: string) {
    await savedSearchesApi.deleteSavedSearch(profileId, searchId);
    searches.value = searches.value.filter((search) => search.id !== searchId);
  }

  async function runSearch(searchId: string) {
    return replace(await savedSearchesApi.runSavedSearch(profileId, searchId));
  }

  onScopeDispose(() => clearTimeout(pollTimer));

  return {
    searches: readonly(searches),
    loading: readonly(loading),
    error: readonly(error),
    fetchSearches,
    createSearch,
    updateSearch,
    deleteSearch,
    runSearch,
  };
}
