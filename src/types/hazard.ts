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
