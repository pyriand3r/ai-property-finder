import { onBeforeUnmount, onMounted, readonly, shallowRef } from 'vue';

import { listListings, reevaluateAll } from '@/api/listings';

import type { TaskEventPayload } from '@/types/task';

export interface ReevaluationProgress {
  done: number;
  total: number;
  failedCount: number;
}

function parseTaskEvent(event: Event): TaskEventPayload | null {
  const messageEvent = event as MessageEvent<string>;
  try {
    return JSON.parse(messageEvent.data) as TaskEventPayload;
  } catch {
    return null;
  }
}

/**
 * Drives the "Bewertung veraltet" banner (ADR 0015): counts listings with
 * needs_reevaluation=true, triggers the bulk re-evaluate endpoint, and
 * tracks its progress over the shared SSE broker (/api/events — the same
 * plumbing Layer 6.3 reuses for chat streaming). Multiple components using
 * this composable for the same profile each open their own EventSource;
 * fine at this scale (single-user, local-network app).
 */
export function useReevaluationBanner(profileId: string) {
  const staleCount = shallowRef(0);
  const running = shallowRef(false);
  const progress = shallowRef<ReevaluationProgress | null>(null);
  const error = shallowRef<string | null>(null);

  let eventSource: EventSource | null = null;
  let activeTaskId: string | null = null;

  async function refreshStaleCount() {
    try {
      const stale = await listListings(profileId, true);
      staleCount.value = stale.length;
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    }
  }

  function handleProgress(payload: TaskEventPayload) {
    if (payload.task_id !== activeTaskId) {
      return;
    }
    progress.value = {
      done: payload.done,
      total: payload.total ?? progress.value?.total ?? 0,
      failedCount: payload.failed_count,
    };
  }

  function handleFinished(payload: TaskEventPayload) {
    if (payload.task_id !== activeTaskId) {
      return;
    }
    running.value = false;
    activeTaskId = null;
    void refreshStaleCount();
  }

  function connect() {
    eventSource = new EventSource('/api/events');
    eventSource.addEventListener('task.progress', (event) => {
      const payload = parseTaskEvent(event);
      if (payload) {
        handleProgress(payload);
      }
    });
    eventSource.addEventListener('task.finished', (event) => {
      const payload = parseTaskEvent(event);
      if (payload) {
        handleFinished(payload);
      }
    });
  }

  async function triggerReevaluateAll() {
    error.value = null;
    running.value = true;
    progress.value = { done: 0, total: staleCount.value, failedCount: 0 };
    try {
      const task = await reevaluateAll(profileId);
      activeTaskId = task.id;
    } catch (triggerError) {
      running.value = false;
      error.value = triggerError instanceof Error ? triggerError.message : String(triggerError);
    }
  }

  onMounted(() => {
    void refreshStaleCount();
    connect();
  });
  onBeforeUnmount(() => {
    eventSource?.close();
  });

  return {
    staleCount: readonly(staleCount),
    running: readonly(running),
    progress: readonly(progress),
    error: readonly(error),
    refreshStaleCount,
    triggerReevaluateAll,
  };
}
