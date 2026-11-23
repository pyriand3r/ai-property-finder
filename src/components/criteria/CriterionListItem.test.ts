import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';

import CriterionListItem from '@/components/criteria/CriterionListItem.vue';

import type { Criterion } from '@/types/criterion';

function criterion(status: Criterion['status']): Criterion {
  return {
    id: 'c1',
    profile_id: 'p1',
    name: 'Nähe zum Wasser',
    description: 'Distanz zum See',
    weight: 3,
    is_subjective: false,
    sort_order: 0,
    status,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  };
}

describe('CriterionListItem', () => {
  it('marks a proposed criterion and emits approve from the Freischalten button', async () => {
    const wrapper = mount(CriterionListItem, { props: { criterion: criterion('proposed') } });

    expect(wrapper.text()).toContain('Vorgeschlagen');
    const approve = wrapper.findAll('button').find((button) => button.text() === 'Freischalten');
    expect(approve).toBeDefined();

    await approve!.trigger('click');
    expect(wrapper.emitted('approve')).toEqual([['c1']]);
  });

  it('shows neither badge nor approve button for an active criterion', () => {
    const wrapper = mount(CriterionListItem, { props: { criterion: criterion('active') } });

    expect(wrapper.text()).not.toContain('Vorgeschlagen');
    expect(wrapper.text()).not.toContain('Freischalten');
  });
});
