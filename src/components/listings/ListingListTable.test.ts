import { flushPromises, mount } from '@vue/test-utils';
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

function makeListing(id: string, status: ListingStatus): Listing {
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
});
