import { http } from '@/api/http';

import type { HazardRegion, RegionHazards } from '@/types/hazard';

/** GET /api/hazards/regions — every Hazard Region, ordered by name (spec 01-13). */
export function listHazardRegions() {
  return http.get<HazardRegion[]>('/api/hazards/regions');
}

/** GET /api/hazards/regions/{key} — one region with its ratings and their sources. */
export function getRegionHazards(regionKey: string) {
  return http.get<RegionHazards>(`/api/hazards/regions/${encodeURIComponent(regionKey)}`);
}
