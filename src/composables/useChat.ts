import { computed, onBeforeUnmount, onMounted, readonly, shallowRef } from 'vue';

import * as chatApi from '@/api/chat';

import type { ChatMessage, ChatSession } from '@/types/chat';

/** Payload shapes for the chat.* SSE events (backend/internal/service
 * /agent_service.go chatTextDeltaPayload/chatMessagePayload
 * /chatTurnErrorPayload — spec 00-03, Layer 6.3). */
interface ChatTextDeltaPayload {
  session_id: string;
  delta: string;
}
interface ChatMessagePayload {
  session_id: string;
  message: ChatMessage;
}
interface ChatTurnErrorPayload {
  session_id: string;
  message: string;
}

function parseSSEPayload<T>(event: Event): T | null {
  const messageEvent = event as MessageEvent<string>;
  try {
    return JSON.parse(messageEvent.data) as T;
  } catch {
    return null;
  }
}

/**
 * Owns a profile's chat sessions + the currently displayed session's
 * messages (spec 01-01), and the Agent turn's live stream (spec 00-03
 * Layer 6.3): sending a message triggers the Agent over SSE
 * (`/api/events`, events `chat.text_delta` / `chat.message`
 * / `chat.turn_error`), and `displayMessages` appends a synthetic
 * in-progress bubble from the accumulating deltas until the final
 * `chat.message` replaces it with the persisted row. The user can browse a
 * past (ended) session via the dropdown, but only the active session
 * accepts new messages — the backend enforces this (409), this composable
 * also exposes `canSend` so the UI can disable the input instead of
 * letting the user hit that error.
 */
export function useChat(profileId: string) {
  const sessions = shallowRef<ChatSession[]>([]);
  const currentSessionId = shallowRef<string | null>(null);
  const messages = shallowRef<ChatMessage[]>([]);
  const loading = shallowRef(false);
  const sending = shallowRef(false);
  const error = shallowRef<string | null>(null);
  const streamingText = shallowRef('');

  let eventSource: EventSource | null = null;

  const activeSession = computed(() => sessions.value.find((session) => !session.ended_at));
  const canSend = computed(
    () => currentSessionId.value !== null && currentSessionId.value === activeSession.value?.id,
  );
  const displayMessages = computed<ChatMessage[]>(() => {
    if (!streamingText.value || !currentSessionId.value) {
      return messages.value;
    }
    return [
      ...messages.value,
      {
        id: '__streaming__',
        session_id: currentSessionId.value,
        role: 'assistant',
        content: streamingText.value,
        created_at: new Date().toISOString(),
      },
    ];
  });

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
    streamingText.value = '';
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
    streamingText.value = '';
  }

  /** Refetches sessions only, to pick up token_count_estimate after a turn
   * ends (the donut chart's data source) — messages are already up to date
   * from the chat.message handler, so fetchSessions()'s extra
   * fetchMessages() call would just be a redundant round trip. */
  async function refreshTokenCount() {
    try {
      sessions.value = await chatApi.listSessions(profileId);
    } catch {
      // Best-effort — the donut just stays stale until the next natural
      // session refresh; not worth surfacing as a user-facing error.
    }
  }

  function connectStream() {
    eventSource = new EventSource('/api/events');
    eventSource.addEventListener('chat.text_delta', (event) => {
      const payload = parseSSEPayload<ChatTextDeltaPayload>(event);
      if (payload && payload.session_id === currentSessionId.value) {
        streamingText.value += payload.delta;
      }
    });
    eventSource.addEventListener('chat.message', (event) => {
      const payload = parseSSEPayload<ChatMessagePayload>(event);
      if (payload && payload.session_id === currentSessionId.value) {
        streamingText.value = '';
        messages.value = [...messages.value, payload.message];
        void refreshTokenCount();
      }
    });
    eventSource.addEventListener('chat.turn_error', (event) => {
      const payload = parseSSEPayload<ChatTurnErrorPayload>(event);
      if (payload && payload.session_id === currentSessionId.value) {
        streamingText.value = '';
        error.value = payload.message;
      }
    });
  }

  onMounted(connectStream);
  onBeforeUnmount(() => eventSource?.close());

  return {
    sessions: readonly(sessions),
    currentSessionId: readonly(currentSessionId),
    activeSession,
    canSend,
    messages: displayMessages,
    loading: readonly(loading),
    sending: readonly(sending),
    error: readonly(error),
    fetchSessions,
    selectSession,
    send,
    startNewSession,
  };
}
