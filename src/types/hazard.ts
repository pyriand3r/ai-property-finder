/** Mirrors backend/internal/domain.HazardType (ADR 0025). Listed in the API's display order. */
export type HazardType = 'wildfire' | 'flood' | 'earthquake' | 'landslide';

/** Mirrors backend/internal/domain.HazardRegion JSON (GET /api/hazards/regions). */
export interface HazardRegion {
  key: string;
  country: string;
  name: string;
  parent_name: string;
}

/** Mirrors backend/internal/domain.HazardSource JSON. */
export interface HazardSource {
  id: string;
  hazard_type: HazardType;
  title: string;
  publisher: string;
  url: string;
  license: string;
  dataset_version: string;
  retrieved_on: string;
  method: string;
  notes: string;
}

/** Mirrors backend/internal/domain.HazardRatingDetail JSON. `rating` is 1-5, higher is worse. */
export interface HazardRatingDetail {
  hazard_type: HazardType;
  rating: number;
  updated_at: string;
  source: HazardSource;
}

/** Mirrors backend/internal/domain.RegionHazards JSON (GET /api/hazards/regions/{key}). */
export interface RegionHazards extends HazardRegion {
  ratings: HazardRatingDetail[];
}

/** Mirrors backend/internal/domain.HazardCatalogEntry JSON (GET /api/hazards/sources, ADR 0027). */
export interface HazardCatalogEntry {
  id: string;
  country: string;
  hazard_type: HazardType;
  /** Part of the country the source covers. */
  scope: string;
  /** `bundled`: the data ships in the app. `candidate`: known, not ingested. */
  status: 'bundled' | 'candidate';
  source_id?: string;
  title: string;
  publisher: string;
  url: string;
  license: string;
  notes: string;
}

export type HazardSuggestionStatus = 'proposed' | 'approved' | 'rejected';

/** Mirrors backend/internal/domain.HazardSourceSuggestion JSON (ADR 0027). */
export interface HazardSourceSuggestion {
  id: string;
  country: string;
  hazard_type: HazardType;
  scope: string;
  title: string;
  publisher: string;
  url: string;
  license: string;
  rationale: string;
  status: HazardSuggestionStatus;
  created_at: string;
  decided_at?: string;
}
