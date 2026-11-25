import type { HazardRegion, HazardType, RegionHazards } from '@/types/hazard';
import type { Listing } from '@/types/listing';

/** Hazard types in the backend's display order (spec 01-13). */
export const hazardTypes: readonly HazardType[] = ['wildfire', 'flood', 'earthquake', 'landslide'];

export const hazardLabel: Record<HazardType, string> = {
  wildfire: 'Waldbrand',
  flood: 'Hochwasser',
  earthquake: 'Erdbeben',
  landslide: 'Erdrutsch',
};

/** A rating is 1 (very low hazard) to 5 (very high). Higher is worse (ADR 0025). */
export const hazardRatingLabel: Record<number, string> = {
  1: 'sehr niedrig',
  2: 'niedrig',
  3: 'mittel',
  4: 'hoch',
  5: 'sehr hoch',
};

/** Rating -> color, green (1) to dark red (5). Same palette family as `gradeColor`, but the
 * direction is the opposite: a high hazard rating is bad, a high Score is good. */
export const hazardRatingColor: Record<number, string> = {
  1: '#18a058',
  2: '#36ad6a',
  3: '#f0a020',
  4: '#d03050',
  5: '#a01030',
};

export const hazardTypeOptions = hazardTypes.map((type) => ({
  label: hazardLabel[type],
  value: type,
}));

export const hazardMaxRatingOptions = [1, 2, 3, 4, 5].map((rating) => ({
  label: `${rating} – ${hazardRatingLabel[rating]}`,
  value: rating,
}));

/** Ratings of one listing's region by hazard type; a type without data is absent. */
export type HazardRatingMap = Partial<Record<HazardType, number>>;

export function ratingMap(region: RegionHazards | undefined): HazardRatingMap {
  const ratings: HazardRatingMap = {};
  for (const rating of region?.ratings ?? []) {
    ratings[rating.hazard_type] = rating.rating;
  }
  return ratings;
}

/** Lower-case, no diacritics, hyphens as spaces: "Évora" and "evora" compare equal. */
export function normalizePlaceName(name: string): string {
  return name
    .normalize('NFD')
    .replace(/\p{M}/gu, '')
    .toLowerCase()
    .replace(/[-–]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

const countryCodeByName: Record<string, string> = {
  portugal: 'PT',
  schweden: 'SE',
  sweden: 'SE',
  sverige: 'SE',
  osterreich: 'AT',
  austria: 'AT',
};

/** ISO alpha-2 code of a free-text `listing.country` ("PT", "Portugal"); null if not recognised. */
function countryCode(country: string | undefined): string | null {
  const normalized = normalizePlaceName(country ?? '');
  if (/^[a-z]{2}$/.test(normalized)) {
    return normalized.toUpperCase();
  }
  return countryCodeByName[normalized] ?? null;
}

/**
 * Finds the Hazard Region of a listing. Nothing in the data model links the two yet (spec
 * 01-13 "Not in this spec", docs/ideas.md), so this matches the free-text `listing.region`
 * against region names, comma/slash-separated parts left to right ("Lisboa, Portugal").
 *
 * - A listing with a recognised `country` only matches regions of that country.
 * - A name that exists more than once ("Lagoa") is resolved by a `parent_name` (district,
 *   island) found in the listing's region or address; if that does not leave exactly one
 *   region, the listing has no region. Guessing would show a wrong rating (spec 01-13: "the
 *   app does not guess").
 */
export function resolveHazardRegion(
  listing: Pick<Listing, 'region' | 'country' | 'address'>,
  regions: readonly HazardRegion[],
): HazardRegion | null {
  if (!listing.region) {
    return null;
  }
  const code = countryCode(listing.country);
  const candidates = code ? regions.filter((region) => region.country === code) : regions;
  const context = normalizePlaceName(`${listing.region} ${listing.address ?? ''}`);

  for (const part of listing.region.split(/[,;/]/)) {
    const name = normalizePlaceName(part);
    if (!name) {
      continue;
    }
    const matches = candidates.filter((region) => normalizePlaceName(region.name) === name);
    if (matches.length === 1) {
      return matches[0] ?? null;
    }
    if (matches.length > 1) {
      const byParent = matches.filter((region) => {
        const parent = normalizePlaceName(region.parent_name);
        return parent !== '' && context.includes(parent);
      });
      return byParent.length === 1 ? (byParent[0] ?? null) : null;
    }
  }
  return null;
}
