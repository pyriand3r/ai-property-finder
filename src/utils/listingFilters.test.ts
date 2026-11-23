import { describe, expect, it } from 'vitest';

import {
  distinctRegions,
  emptyListingFilters,
  matchesListingFilters,
} from '@/utils/listingFilters';

import type { Listing } from '@/types/listing';

function makeListing(overrides: Partial<Listing> = {}): Listing {
  return {
    id: 'a',
    profile_id: 'p1',
    title: 'Haus',
    status: 'new',
    source_type: 'manual',
    user_confirmed_fields: [],
    needs_reevaluation: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

describe('matchesListingFilters', () => {
  it('matches everything, including listings without values, when no filter is set', () => {
    expect(matchesListingFilters(makeListing(), emptyListingFilters())).toBe(true);
  });

  it('treats range bounds as inclusive', () => {
    const filters = { ...emptyListingFilters(), price: { min: 100, max: 200 } };
    expect(matchesListingFilters(makeListing({ price: 100 }), filters)).toBe(true);
    expect(matchesListingFilters(makeListing({ price: 200 }), filters)).toBe(true);
    expect(matchesListingFilters(makeListing({ price: 99 }), filters)).toBe(false);
    expect(matchesListingFilters(makeListing({ price: 201 }), filters)).toBe(false);
  });

  it('supports a single open bound', () => {
    const minOnly = { ...emptyListingFilters(), size: { min: 80, max: null } };
    expect(matchesListingFilters(makeListing({ property_size_m2: 1000 }), minOnly)).toBe(true);
    expect(matchesListingFilters(makeListing({ property_size_m2: 79 }), minOnly)).toBe(false);

    const maxOnly = { ...emptyListingFilters(), score: { min: null, max: 60 } };
    expect(matchesListingFilters(makeListing({ overall_score: 0 }), maxOnly)).toBe(true);
    expect(matchesListingFilters(makeListing({ overall_score: 61 }), maxOnly)).toBe(false);
  });

  it('hides listings with an unknown value once a bound on that field is active', () => {
    const filters = { ...emptyListingFilters(), price: { min: null, max: 300000 } };
    expect(matchesListingFilters(makeListing({ price: undefined }), filters)).toBe(false);
  });

  it('keeps a score of 0 inside a range that includes 0', () => {
    const filters = { ...emptyListingFilters(), score: { min: 0, max: 10 } };
    expect(matchesListingFilters(makeListing({ overall_score: 0 }), filters)).toBe(true);
  });

  it('filters by region and ignores surrounding whitespace in the stored value', () => {
    const filters = { ...emptyListingFilters(), region: 'Dalarna' };
    expect(matchesListingFilters(makeListing({ region: ' Dalarna ' }), filters)).toBe(true);
    expect(matchesListingFilters(makeListing({ region: 'Värmland' }), filters)).toBe(false);
    expect(matchesListingFilters(makeListing(), filters)).toBe(false);
  });

  it('combines all filters with AND', () => {
    const filters = {
      ...emptyListingFilters(),
      status: 'visited' as const,
      region: 'Dalarna',
      price: { min: null, max: 200000 },
    };
    const hit = makeListing({ status: 'visited', region: 'Dalarna', price: 150000 });
    expect(matchesListingFilters(hit, filters)).toBe(true);
    expect(matchesListingFilters({ ...hit, status: 'new' }, filters)).toBe(false);
    expect(matchesListingFilters({ ...hit, price: 250000 }, filters)).toBe(false);
  });
});

describe('distinctRegions', () => {
  it('returns sorted unique non-blank regions', () => {
    const listings = [
      makeListing({ id: '1', region: 'Värmland' }),
      makeListing({ id: '2', region: 'Dalarna' }),
      makeListing({ id: '3', region: 'Dalarna ' }),
      makeListing({ id: '4', region: '  ' }),
      makeListing({ id: '5' }),
    ];
    expect(distinctRegions(listings)).toEqual(['Dalarna', 'Värmland']);
  });
});
