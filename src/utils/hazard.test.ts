import { describe, expect, it } from 'vitest';

import { normalizePlaceName, ratingMap, resolveHazardRegion } from '@/utils/hazard';

import type { HazardRegion, RegionHazards } from '@/types/hazard';

const regions: HazardRegion[] = [
  { key: 'PT-1106', country: 'PT', name: 'Lisboa', parent_name: 'Lisboa' },
  { key: 'PT-0806', country: 'PT', name: 'Lagoa', parent_name: 'Faro' },
  { key: 'PT-4201', country: 'PT', name: 'Lagoa', parent_name: 'Ilha de São Miguel' },
  { key: 'PT-0701', country: 'PT', name: 'Évora', parent_name: 'Évora' },
  { key: 'SE-0001', country: 'SE', name: 'Lisboa', parent_name: 'Testlän' },
];

describe('resolveHazardRegion', () => {
  it('matches a region name ignoring case and accents', () => {
    expect(resolveHazardRegion({ region: 'evora' }, regions)?.key).toBe('PT-0701');
  });

  it('finds the region in a comma-separated value', () => {
    expect(resolveHazardRegion({ region: 'Lisboa, Portugal' }, regions)?.key).toBe('PT-1106');
  });

  it('restricts to the listing country, by code or by name', () => {
    expect(resolveHazardRegion({ region: 'Lisboa', country: 'Portugal' }, regions)?.key).toBe(
      'PT-1106',
    );
    expect(resolveHazardRegion({ region: 'Lisboa', country: 'se' }, regions)?.key).toBe('SE-0001');
  });

  /** Spec 01-13: "Lagoa" exists twice; a wrong rating is worse than none. */
  it('does not guess between regions with the same name', () => {
    expect(resolveHazardRegion({ region: 'Lagoa', country: 'PT' }, regions)).toBeNull();
  });

  it('resolves a duplicate name by the parent named in the region or address', () => {
    expect(
      resolveHazardRegion({ region: 'Lagoa', country: 'PT', address: 'Rua X, Faro' }, regions)?.key,
    ).toBe('PT-0806');
    expect(
      resolveHazardRegion({ region: 'Lagoa, Ilha de São Miguel', country: 'PT' }, regions)?.key,
    ).toBe('PT-4201');
  });

  it('returns null for unknown or missing regions', () => {
    expect(resolveHazardRegion({ region: 'Alentejo' }, regions)).toBeNull();
    expect(resolveHazardRegion({}, regions)).toBeNull();
  });
});

describe('normalizePlaceName', () => {
  it('drops diacritics and treats hyphens as spaces', () => {
    expect(normalizePlaceName(' Vila-Nova  de Gaia ')).toBe('vila nova de gaia');
    expect(normalizePlaceName('Österreich')).toBe('osterreich');
  });
});

describe('ratingMap', () => {
  it('maps hazard type to rating and leaves types without data absent', () => {
    const region = {
      ratings: [{ hazard_type: 'earthquake', rating: 3 }],
    } as unknown as RegionHazards;
    expect(ratingMap(region)).toEqual({ earthquake: 3 });
    expect(ratingMap(undefined)).toEqual({});
  });
});
