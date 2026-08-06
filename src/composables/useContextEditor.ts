import { readonly, shallowRef } from 'vue';

import type { ContextFile, ContextRevision } from '@/types/context';

/** Drives a Context Editor (Global or Profile, spec 01-02) — both scopes
 * share identical load/save/history/restore logic, just against different
 * endpoints, so the four operations are injected rather than duplicating
 * this composable per scope. */
export function useContextEditor(
  fetchContext: () => Promise<ContextFile>,
  updateContext: (contentMd: string) => Promise<ContextFile>,
  fetchHistory: () => Promise<ContextRevision[]>,
  restoreRevision: (revisionId: string) => Promise<ContextFile>,
) {
  const context = shallowRef<ContextFile | null>(null);
  const history = shallowRef<ContextRevision[]>([]);
  const loading = shallowRef(false);
  const saving = shallowRef(false);
  const error = shallowRef<string | null>(null);

  async function fetchAll() {
    loading.value = true;
    error.value = null;
    try {
      const [fetchedContext, fetchedHistory] = await Promise.all([fetchContext(), fetchHistory()]);
      context.value = fetchedContext;
      history.value = fetchedHistory;
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    } finally {
      loading.value = false;
    }
  }

  async function save(contentMd: string) {
    saving.value = true;
    error.value = null;
    try {
      context.value = await updateContext(contentMd);
      history.value = await fetchHistory();
    } catch (saveError) {
      error.value = saveError instanceof Error ? saveError.message : String(saveError);
      throw saveError;
    } finally {
      saving.value = false;
    }
  }

  async function restore(revisionId: string) {
    saving.value = true;
    error.value = null;
    try {
      context.value = await restoreRevision(revisionId);
      history.value = await fetchHistory();
    } catch (restoreError) {
      error.value = restoreError instanceof Error ? restoreError.message : String(restoreError);
      throw restoreError;
    } finally {
      saving.value = false;
    }
  }

  return {
    context: readonly(context),
    history: readonly(history),
    loading: readonly(loading),
    saving: readonly(saving),
    error: readonly(error),
    fetchAll,
    save,
    restore,
  };
}
