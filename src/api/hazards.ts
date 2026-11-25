import { http } from '@/api/http';

import type {
  HazardCatalogEntry,
  HazardRegion,
  HazardSourceSuggestion,
  RegionHazards,
} from '@/types/hazard';

/** GET /api/hazards/regions — every Hazard Region, ordered by name (spec 01-13). */
export function listHazardRegions() {
  return http.get<HazardRegion[]>('/api/hazards/regions');
}

/** GET /api/hazards/regions/{key} — one region with its ratings and their sources. */
export function getRegionHazards(regionKey: string) {
  return http.get<RegionHazards>(`/api/hazards/regions/${encodeURIComponent(regionKey)}`);
}

/** GET /api/hazards/sources — the source catalogue, bundled and candidate (ADR 0027). */
export function listHazardSources() {
  return http.get<HazardCatalogEntry[]>('/api/hazards/sources');
}

/** GET /api/hazards/source-suggestions — sources the Agent suggested, newest first. */
export function listSourceSuggestions() {
  return http.get<HazardSourceSuggestion[]>('/api/hazards/source-suggestions');
}

/** Clears a proposed source for ingestion. It does not ingest anything itself (ADR 0027). */
export function approveSourceSuggestion(suggestionId: string) {
  return http.post<HazardSourceSuggestion>(
    `/api/hazards/source-suggestions/${suggestionId}/approve`,
  );
}

/** Marks a proposed source as unwanted; the same source is not suggested again. */
export function rejectSourceSuggestion(suggestionId: string) {
  return http.post<HazardSourceSuggestion>(
    `/api/hazards/source-suggestions/${suggestionId}/reject`,
  );
}
