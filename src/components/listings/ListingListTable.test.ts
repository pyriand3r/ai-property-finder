import { flushPromises, mount } from '@vue/test-utils';
import { NSelect } from 'naive-ui';
import { describe, expect, it, vi } from 'vitest';
import { shallowRef } from 'vue';

import ListingListTable from '@/components/listings/ListingListTable.vue';

import type { Listing, ListingStatus } from '@/types/listing';

// The table's own rendering is under test; its data composables/API call /api.
vi.mock('@/api/criteria', () => ({ listCriteria: () => Promise.resolve([]) }));
vi.mock('@/composables/useListingThumbnails', () => ({
  useListingThumbnails: () => ({ thumbnails: shallowRef(new Map()) }),
}));
vi.mock('@/composables/useListingGrades', () => ({
  useListingGrades: () => ({ grades: shallowRef(new Map()) }),
}));

function makeListing(id: string, status: ListingStatus, overrides: Partial<Listing> = {}): Listing {
  return {
    id,
    profile_id: 'p1',
    title: `Haus ${id}`,
    status,
    source_type: 'manual',
    user_confirmed_fields: [],
    needs_reevaluation: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

describe('ListingListTable', () => {
  it('shows German status labels, not the raw enum values', async () => {
    const wrapper = mount(ListingListTable, {
      props: {
        profileId: 'p1',
        listings: [makeListing('a', 'in_negotiation'), makeListing('b', 'new')],
      },
    });
    await flushPromises();

    const tags = wrapper.findAll('.n-tag').map((tag) => tag.text());
    expect(tags).toContain('In Verhandlung');
    expect(tags).toContain('Neu');
    expect(wrapper.text()).not.toContain('in_negotiation');
  });

  it('narrows the rows by a price range entered in the filter row', async () => {
    const wrapper = mount(ListingListTable, {
      props: {
        profileId: 'p1',
        listings: [
          makeListing('cheap', 'new', { price: 100000 }),
          makeListing('mid', 'new', { price: 200000 }),
          makeListing('dear', 'new', { price: 400000 }),
        ],
      },
    });
    await flushPromises();
    expect(wrapper.text()).toContain('Haus cheap');

    const min = wrapper.find('[data-testid="filter-price-min"] input');
    await min.setValue('150000');
    await min.trigger('change');
    await min.trigger('blur');
    const max = wrapper.find('[data-testid="filter-price-max"] input');
    await max.setValue('300000');
    await max.trigger('change');
    await max.trigger('blur');
    await flushPromises();

    expect(wrapper.text()).not.toContain('Haus cheap');
    expect(wrapper.text()).toContain('Haus mid');
    expect(wrapper.text()).not.toContain('Haus dear');
  });

  it('offers the distinct regions as options and filters the rows by the chosen one', async () => {
    const wrapper = mount(ListingListTable, {
      props: {
        profileId: 'p1',
        listings: [
          makeListing('a', 'new', { region: 'Dalarna' }),
          makeListing('b', 'new', { region: 'Dalarna' }),
          makeListing('c', 'new', { region: 'Värmland' }),
        ],
      },
    });
    await flushPromises();

    // Driven through the component API: the dropdown popup needs layout APIs jsdom lacks.
    const regionSelect = wrapper
      .findAllComponents(NSelect)
      .find((select) => select.props('placeholder') === 'Region');
    expect(regionSelect?.props('options')).toEqual([
      { label: 'Dalarna', value: 'Dalarna' },
      { label: 'Värmland', value: 'Värmland' },
    ]);

    regionSelect?.vm.$emit('update:value', 'Värmland');
    await flushPromises();

    expect(wrapper.text()).toContain('Haus c');
    expect(wrapper.text()).not.toContain('Haus a');
    expect(wrapper.text()).not.toContain('Haus b');
  });
});
