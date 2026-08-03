import { computed, readonly, shallowRef } from 'vue';

import * as chatApi from '@/api/chat';

import type { ChatMessage, ChatSession } from '@/types/chat';

/**
 * Owns a profile's chat sessions + the currently displayed session's
 * messages (spec 01-01 Layer 6.1 — persistence only, no Agent turn yet).
 * The user can browse a past (ended) session via the dropdown, but only
 * the active session accepts new messages — the backend enforces this
 * (409), this composable also exposes `canSend` so the UI can disable the
 * input instead of letting the user hit that error.
 */
export function useChat(profileId: string) {
  const sessions = shallowRef<ChatSession[]>([]);
  const currentSessionId = shallowRef<string | null>(null);
  const messages = shallowRef<ChatMessage[]>([]);
  const loading = shallowRef(false);
  const sending = shallowRef(false);
  const error = shallowRef<string | null>(null);

  const activeSession = computed(() => sessions.value.find((session) => !session.ended_at));
  const canSend = computed(
    () => currentSessionId.value !== null && currentSessionId.value === activeSession.value?.id,
  );

  async function fetchSessions() {
    loading.value = true;
    error.value = null;
    try {
      sessions.value = await chatApi.listSessions(profileId);
      currentSessionId.value ??= activeSession.value?.id ?? sessions.value[0]?.id ?? null;
      if (currentSessionId.value) {
        await fetchMessages();
      }
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    } finally {
      loading.value = false;
    }
  }

  async function fetchMessages() {
    if (!currentSessionId.value) {
      return;
    }
    try {
      messages.value = await chatApi.listMessages(profileId, currentSessionId.value);
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    }
  }

  async function selectSession(sessionId: string) {
    currentSessionId.value = sessionId;
    await fetchMessages();
  }

  async function send(content: string, attachmentPath?: string) {
    if (!currentSessionId.value) {
      return;
    }
    sending.value = true;
    error.value = null;
    try {
      const message = await chatApi.sendMessage(
        profileId,
        currentSessionId.value,
        content,
        attachmentPath,
      );
      messages.value = [...messages.value, message];
    } catch (sendError) {
      error.value = sendError instanceof Error ? sendError.message : String(sendError);
      throw sendError;
    } finally {
      sending.value = false;
    }
  }

  async function startNewSession() {
    const session = await chatApi.startSession(profileId);
    sessions.value = [
      session,
      ...sessions.value.map((s) => (s.ended_at ? s : { ...s, ended_at: session.created_at })),
    ];
    currentSessionId.value = session.id;
    messages.value = [];
  }

  return {
    sessions: readonly(sessions),
    currentSessionId: readonly(currentSessionId),
    activeSession,
    canSend,
    messages: readonly(messages),
    loading: readonly(loading),
    sending: readonly(sending),
    error: readonly(error),
    fetchSessions,
    selectSession,
    send,
    startNewSession,
  };
}
