import { flushPromises, mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';

import ListingHazards from '@/components/listings/ListingHazards.vue';

import type { HazardRegion, RegionHazards } from '@/types/hazard';

const regions: HazardRegion[] = [
  { key: 'PT-1106', country: 'PT', name: 'Lisboa', parent_name: 'Lisboa' },
];

const lisboa: RegionHazards = {
  ...regions[0]!,
  ratings: [
    {
      hazard_type: 'earthquake',
      rating: 3,
      updated_at: '2026-11-24T06:46:18+01:00',
      source: {
        id: 'pt-np-en-1998-1-seismic-zones',
        hazard_type: 'earthquake',
        title: 'NP EN 1998-1',
        publisher: 'IPQ',
        url: 'https://example.org/eq',
        license: 'x',
        dataset_version: '2010',
        retrieved_on: '2026-11-24',
        method: 'agR-Stufen',
        notes: '',
      },
    },
  ],
};

vi.mock('@/api/hazards', () => ({
  listHazardRegions: () => Promise.resolve(regions),
  getRegionHazards: (key: string) =>
    key === 'PT-1106' ? Promise.resolve(lisboa) : Promise.reject(new Error('404')),
}));

describe('ListingHazards', () => {
  it('lists every hazard type, with a German rating badge where data exists and a gap note where not', async () => {
    const wrapper = mount(ListingHazards, {
      props: { listing: { id: 'a', region: 'Lisboa', country: 'Portugal' } },
    });
    await flushPromises();

    expect(wrapper.find('[data-testid="hazard-badge-earthquake"]').text()).toBe('3 – mittel');
    expect(wrapper.text()).toContain('Waldbrand');
    expect(wrapper.text()).toContain('Erdrutsch');
    expect(wrapper.text()).toContain('Keine Daten für diese Region');
    expect(wrapper.text()).toContain('fließt nicht in den Score ein');
  });

  it('says so when the listing region is not a known hazard region', async () => {
    const wrapper = mount(ListingHazards, {
      props: { listing: { id: 'b', region: 'Alentejo' } },
    });
    await flushPromises();

    expect(wrapper.text()).toContain('Keine Gefahrendaten');
    expect(wrapper.find('[data-testid="hazard-badge-earthquake"]').exists()).toBe(false);
  });
});
