import type { Listing, ListingStatus } from '@/types/listing';

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
}

export function emptyListingFilters(): ListingFilters {
  return {
    status: null,
    score: { min: null, max: null },
    price: { min: null, max: null },
    size: { min: null, max: null },
    region: null,
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

export function matchesListingFilters(listing: Listing, filters: ListingFilters): boolean {
  return (
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
