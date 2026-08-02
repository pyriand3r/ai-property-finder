import { http } from '@/api/http';

import type { Listing } from '@/types/listing';
import type { Profile } from '@/types/profile';

export interface RankingRefreshResponse {
  profile: Profile;
  listings: Listing[];
}

/** POST /api/profiles/{id}/ranking/refresh — spec 01-05 Layer 5.2. One
 * isolated LLM call (no Agent) regenerating every rankable listing's
 * kurzfazit plus the profile's overall ranking_summary. */
export function refreshRanking(profileId: string) {
  return http.post<RankingRefreshResponse>(`/api/profiles/${profileId}/ranking/refresh`);
}
