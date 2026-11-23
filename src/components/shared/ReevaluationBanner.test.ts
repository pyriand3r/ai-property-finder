import { mount } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { shallowRef } from 'vue';

import ReevaluationBanner from '@/components/shared/ReevaluationBanner.vue';

import type { ReevaluationProgress } from '@/composables/useReevaluationBanner';

const state = {
  staleCount: shallowRef(0),
  running: shallowRef(false),
  progress: shallowRef<ReevaluationProgress | null>(null),
  error: shallowRef<string | null>(null),
  triggerReevaluateAll: vi.fn(),
};

// The composable talks to /api and EventSource; the banner's own branching is what is tested.
vi.mock('@/composables/useReevaluationBanner', () => ({
  useReevaluationBanner: () => state,
}));

beforeEach(() => {
  state.staleCount.value = 0;
  state.running.value = false;
  state.progress.value = null;
  state.error.value = null;
  state.triggerReevaluateAll.mockReset();
});

describe('ReevaluationBanner', () => {
  it('renders nothing when no listing is stale', () => {
    const wrapper = mount(ReevaluationBanner, { props: { profileId: 'p1' } });
    expect(wrapper.text()).toBe('');
  });

  it('offers bulk re-evaluation with a singular count and triggers it on click', async () => {
    state.staleCount.value = 1;
    const wrapper = mount(ReevaluationBanner, { props: { profileId: 'p1' } });
    expect(wrapper.text()).toContain('1 Inserat neu bewerten?');

    await wrapper.find('button').trigger('click');
    expect(state.triggerReevaluateAll).toHaveBeenCalledOnce();
  });

  it('uses the plural form for several stale listings', () => {
    state.staleCount.value = 3;
    const wrapper = mount(ReevaluationBanner, { props: { profileId: 'p1' } });
    expect(wrapper.text()).toContain('3 Inserate neu bewerten?');
  });

  it('shows progress while running, taking precedence over the stale prompt', () => {
    state.staleCount.value = 3;
    state.running.value = true;
    state.progress.value = { done: 2, total: 4, failedCount: 1 };
    const wrapper = mount(ReevaluationBanner, { props: { profileId: 'p1' } });
    expect(wrapper.text()).toContain('Neubewertung läuft');
    expect(wrapper.text()).toContain('2 von 4 erledigt');
    expect(wrapper.text()).toContain('1 Fehler');
    expect(wrapper.find('button').exists()).toBe(false);
  });

  it('shows an error before anything else', () => {
    state.staleCount.value = 3;
    state.running.value = true;
    state.error.value = 'Backend nicht erreichbar';
    const wrapper = mount(ReevaluationBanner, { props: { profileId: 'p1' } });
    expect(wrapper.text()).toContain('Backend nicht erreichbar');
    expect(wrapper.text()).not.toContain('Neubewertung läuft');
  });
});
