import { http } from '@/api/http';

import type { SavedSearch } from '@/types/savedSearch';

/** The editable fields of a saved search; omitted ones get the backend's defaults on create. */
export interface SavedSearchInput {
  name?: string;
  url?: string;
  page_param?: string;
  max_pages?: number;
  link_contains?: string;
  interval_hours?: number;
  enabled?: boolean;
  import_new?: boolean;
  max_import?: number;
}

/** GET /api/profiles/{id}/saved-searches (spec 01-17). */
export function listSavedSearches(profileId: string) {
  return http.get<SavedSearch[]>(`/api/profiles/${profileId}/saved-searches`);
}

export function createSavedSearch(profileId: string, input: SavedSearchInput) {
  return http.post<SavedSearch>(`/api/profiles/${profileId}/saved-searches`, input);
}

export function updateSavedSearch(profileId: string, searchId: string, input: SavedSearchInput) {
  return http.patch<SavedSearch>(`/api/profiles/${profileId}/saved-searches/${searchId}`, input);
}

export function deleteSavedSearch(profileId: string, searchId: string) {
  return http.delete<void>(`/api/profiles/${profileId}/saved-searches/${searchId}`);
}

/** Starts a run now (202). The outcome shows in the search once the run is done. */
export function runSavedSearch(profileId: string, searchId: string) {
  return http.post<SavedSearch>(`/api/profiles/${profileId}/saved-searches/${searchId}/run`);
}
