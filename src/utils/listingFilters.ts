import type { HazardType } from '@/types/hazard';
import type { Listing, ListingStatus } from '@/types/listing';
import type { HazardRatingMap } from '@/utils/hazard';

/** An inclusive numeric range; `null` = that bound is not set. */
export interface NumberRange {
  min: number | null;
  max: number | null;
}

/** List View filter state (spec 01-05 § List View). All filters are client-side. */
export interface ListingFilters {
  status: ListingStatus | null;
  score: NumberRange;
  price: NumberRange;
  size: NumberRange;
  region: string | null;
  hazard: HazardFilter;
}

/**
 * "Waldbrand höchstens 3": keeps listings whose region rating for `type` is at most `max`.
 * Higher is worse (ADR 0025). Active only when both are set.
 */
export interface HazardFilter {
  type: HazardType | null;
  max: number | null;
}

export function emptyListingFilters(): ListingFilters {
  return {
    status: null,
    score: { min: null, max: null },
    price: { min: null, max: null },
    size: { min: null, max: null },
    region: null,
    hazard: { type: null, max: null },
  };
}

/**
 * A Listing without a value never matches an active bound: "price max. 300k"
 * must not show listings whose price is unknown. Bounds are inclusive.
 */
function inRange(value: number | undefined, range: NumberRange): boolean {
  if (range.min === null && range.max === null) {
    return true;
  }
  if (value === undefined) {
    return false;
  }
  return (range.min === null || value >= range.min) && (range.max === null || value <= range.max);
}

/**
 * Like a numeric range, a listing without a rating (no region found, or no data for that
 * hazard) is hidden once the hazard filter is active: "Erdbeben höchstens 2" must not show
 * listings whose earthquake hazard is unknown.
 */
function matchesHazard(filter: HazardFilter, ratings: HazardRatingMap | undefined): boolean {
  if (filter.type === null || filter.max === null) {
    return true;
  }
  const rating = ratings?.[filter.type];
  return rating !== undefined && rating <= filter.max;
}

/** `hazardRatings` = the ratings of the listing's Hazard Region, if one was found. */
export function matchesListingFilters(
  listing: Listing,
  filters: ListingFilters,
  hazardRatings?: HazardRatingMap,
): boolean {
  return (
    matchesHazard(filters.hazard, hazardRatings) &&
    (filters.status === null || listing.status === filters.status) &&
    inRange(listing.overall_score, filters.score) &&
    inRange(listing.price, filters.price) &&
    inRange(listing.property_size_m2, filters.size) &&
    (filters.region === null || listing.region?.trim() === filters.region)
  );
}

/** Distinct non-blank `region` values, sorted for the region dropdown. */
export function distinctRegions(listings: readonly Listing[]): string[] {
  const regions = new Set<string>();
  for (const listing of listings) {
    const region = listing.region?.trim();
    if (region) {
      regions.add(region);
    }
  }
  // Sorting a fresh copy; toSorted() is not in the ES2022 lib this project targets.
  // oxlint-disable-next-line unicorn/no-array-sort
  return [...regions].sort((a, b) => a.localeCompare(b, 'de'));
}
