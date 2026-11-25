import { flushPromises, mount } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { shallowRef } from 'vue';

import SavedSearchesPanel from '@/components/searches/SavedSearchesPanel.vue';

import type { SavedSearch } from '@/types/savedSearch';
import type * as naive from 'naive-ui';

const state = {
  searches: shallowRef<SavedSearch[]>([]),
  loading: shallowRef(false),
  error: shallowRef<string | null>(null),
  fetchSearches: vi.fn(),
  createSearch: vi.fn(),
  updateSearch: vi.fn(),
  deleteSearch: vi.fn(),
  runSearch: vi.fn(),
};

// The composable talks to /api; the panel's own branching is what is tested.
vi.mock('@/composables/useSavedSearches', () => ({ useSavedSearches: () => state }));

const messageError = vi.fn();
vi.mock('naive-ui', async (importOriginal) => ({
  ...(await importOriginal<typeof naive>()),
  useMessage: () => ({ error: messageError }),
}));

function savedSearch(overrides: Partial<SavedSearch>): SavedSearch {
  return {
    id: 's1',
    profile_id: 'p1',
    name: 'Algarve',
    url: 'https://portal.test/search',
    page_param: '',
    max_pages: 1,
    link_contains: '',
    interval_hours: 24,
    enabled: true,
    import_new: true,
    max_import: 3,
    last_status: 'idle',
    last_found: 0,
    last_new: 0,
    last_imported: 0,
    last_summary: '',
    created_at: '2026-11-25T10:00:00Z',
    updated_at: '2026-11-25T10:00:00Z',
    ...overrides,
  };
}

function mountPanel() {
  return mount(SavedSearchesPanel, { props: { profileId: 'p1' } });
}

describe('SavedSearchesPanel', () => {
  beforeEach(() => {
    state.searches.value = [];
    state.loading.value = false;
    state.error.value = null;
    state.fetchSearches.mockReset();
    state.runSearch.mockReset();
    messageError.mockReset();
  });

  it('loads on mount and shows a hint while there are no searches', () => {
    const wrapper = mountPanel();
    expect(state.fetchSearches).toHaveBeenCalled();
    expect(wrapper.text()).toContain('Noch keine gespeicherten Suchen.');
  });

  it('shows schedule, import setting and the outcome of the last run', () => {
    state.searches.value = [
      savedSearch({
        last_status: 'succeeded',
        last_run_at: '2026-11-25T10:00:00Z',
        last_summary: '4 Treffer, 3 neu, 2 an den Agent übergeben',
      }),
    ];
    const text = mountPanel().text();
    expect(text).toContain('Algarve');
    expect(text).toContain('täglich');
    expect(text).toContain('bis zu 3 pro Lauf');
    expect(text).toContain('Erfolgreich');
    expect(text).toContain('4 Treffer, 3 neu, 2 an den Agent übergeben');
  });

  it('shows the error of a failed run instead of the summary', () => {
    state.searches.value = [
      savedSearch({ last_status: 'failed', last_error: 'Seite 1 konnte nicht geladen werden' }),
    ];
    expect(mountPanel().text()).toContain('Seite 1 konnte nicht geladen werden');
  });

  it('starts a run, and refuses to start a second one while it runs', async () => {
    state.searches.value = [
      savedSearch({ id: 'a' }),
      savedSearch({ id: 'b', last_status: 'running' }),
    ];
    const wrapper = mountPanel();
    const buttons = wrapper.findAll('button').filter((button) => button.text() === 'Jetzt suchen');
    expect(buttons).toHaveLength(2);
    expect(buttons[1]!.attributes('disabled')).toBeDefined();

    await buttons[0]!.trigger('click');
    expect(state.runSearch).toHaveBeenCalledWith('a');
  });

  it('shows a failed action as a toast', async () => {
    state.searches.value = [savedSearch({})];
    state.runSearch.mockRejectedValue(new Error('Die Suche läuft bereits'));
    const wrapper = mountPanel();
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Jetzt suchen')!
      .trigger('click');
    await flushPromises();
    expect(messageError).toHaveBeenCalledWith('Die Suche läuft bereits');
  });
});
