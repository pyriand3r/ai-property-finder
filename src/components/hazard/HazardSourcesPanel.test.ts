import { mount } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { shallowRef } from 'vue';

import HazardSourcesPanel from '@/components/hazard/HazardSourcesPanel.vue';

import type { HazardCatalogEntry, HazardSourceSuggestion } from '@/types/hazard';
import type * as naive from 'naive-ui';

const state = {
  catalog: shallowRef<HazardCatalogEntry[]>([]),
  suggestions: shallowRef<HazardSourceSuggestion[]>([]),
  loading: shallowRef(false),
  error: shallowRef<string | null>(null),
  fetchAll: vi.fn(),
  approve: vi.fn(),
  reject: vi.fn(),
};

// The composable talks to /api; the panel's own branching is what is tested.
vi.mock('@/composables/useHazardSources', () => ({ useHazardSources: () => state }));

const messageError = vi.fn();
vi.mock('naive-ui', async (importOriginal) => ({
  ...(await importOriginal<typeof naive>()),
  useMessage: () => ({ error: messageError }),
}));

function suggestion(overrides: Partial<HazardSourceSuggestion>): HazardSourceSuggestion {
  return {
    id: 's1',
    country: 'SE',
    hazard_type: 'earthquake',
    scope: '',
    title: 'SNSN',
    publisher: 'Uppsala universitet',
    url: 'https://snsn.se',
    license: '',
    rationale: 'Erdbebenkatalog',
    status: 'proposed',
    created_at: '2026-11-25T10:00:00Z',
    ...overrides,
  };
}

function entry(overrides: Partial<HazardCatalogEntry>): HazardCatalogEntry {
  return {
    id: 'pt-flood-apa',
    country: 'PT',
    hazard_type: 'flood',
    scope: 'Portugal (Festland)',
    status: 'bundled',
    title: 'PGRI 2',
    publisher: 'APA',
    url: 'https://apambiente.pt',
    license: 'x',
    notes: '',
    ...overrides,
  };
}

beforeEach(() => {
  state.catalog.value = [];
  state.suggestions.value = [];
  state.error.value = null;
  state.fetchAll.mockReset();
  state.approve.mockReset().mockResolvedValue(undefined);
  state.reject.mockReset().mockResolvedValue(undefined);
  messageError.mockReset();
});

describe('HazardSourcesPanel', () => {
  it('loads on mount and lets the user approve or reject a proposed suggestion', async () => {
    state.suggestions.value = [suggestion({ id: 's1' })];
    const wrapper = mount(HazardSourcesPanel);
    expect(state.fetchAll).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain('SNSN');

    const buttons = wrapper.findAll('button');
    await buttons.find((b) => b.text() === 'Freigeben')!.trigger('click');
    expect(state.approve).toHaveBeenCalledWith('s1');
    await buttons.find((b) => b.text() === 'Ablehnen')!.trigger('click');
    expect(state.reject).toHaveBeenCalledWith('s1');
  });

  it('shows a decided suggestion without buttons and says an approval is not an ingestion', () => {
    state.suggestions.value = [suggestion({ status: 'approved' })];
    const wrapper = mount(HazardSourcesPanel);
    expect(wrapper.text()).toContain('Freigegeben – wartet auf Einbindung im nächsten Release');
    expect(wrapper.findAll('button')).toHaveLength(0);
  });

  it('toasts the error when a decision fails', async () => {
    state.suggestions.value = [suggestion({})];
    state.approve.mockRejectedValue(new Error('Der Vorschlag ist schon entschieden'));
    const wrapper = mount(HazardSourcesPanel);
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Freigeben')!
      .trigger('click');
    await Promise.resolve();
    expect(messageError).toHaveBeenCalledWith('Der Vorschlag ist schon entschieden');
  });

  it('marks catalogue entries as shipped or not ingested', () => {
    state.catalog.value = [
      entry({}),
      entry({
        id: 'at-flood-hora',
        country: 'AT',
        status: 'candidate',
        title: 'HORA',
        notes: 'Lizenz offen',
      }),
    ];
    const text = mount(HazardSourcesPanel).text();
    expect(text).toContain('Eingebunden');
    expect(text).toContain('Nicht eingebunden');
    expect(text).toContain('Lizenz offen');
  });
});
