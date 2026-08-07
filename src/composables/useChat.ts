import { computed, onBeforeUnmount, onMounted, readonly, shallowRef } from 'vue';

import * as chatApi from '@/api/chat';
import { ApiError } from '@/api/http';
import { hasPendingConfirmCard } from '@/utils/chat';

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
}
interface ChatTurnErrorPayload {
  session_id: string;
  message: string;
}
interface ChatToolProgressPayload {
  session_id: string;
  tool: string;
}

/** German progress labels for the tools a Tool call in progress can name
 * (spec 00-03: "each intermediate tool call is also streamed as a
 * progress event so the user sees 'Lese Seite…', 'Extrahiere Bilder…'
 * live"). Falls back to the raw tool name for any tool added later without
 * a label here. */
const toolProgressLabels: Record<string, string> = {
  fetch_url: 'Lese Seite…',
  save_listing: 'Speichere Inserat…',
  download_image: 'Lade Bild herunter…',
};

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
 * Layer 6.3/6.5): sending a message triggers the Agent over SSE
 * (`/api/events`, events `chat.text_delta` / `chat.tool_progress`
 * / `chat.message` / `chat.turn_error`), and `displayMessages` appends a
 * synthetic in-progress bubble from the accumulating deltas/tool-progress
 * label until the turn ends. `chat.message` only carries a session_id —
 * every roundtrip of the turn persisted its own row (Layer 6.5, so
 * pending-confirmation tool calls from an intermediate roundtrip are
 * readable too, not just the final reply), so on turn-end the handler
 * refetches the full message list rather than appending one row. The user
 * can browse a past (ended) session via the dropdown, but only the active
 * session accepts new messages — the backend enforces this (409), this
 * composable also exposes `canSend` so the UI can disable the input
 * instead of letting the user hit that error.
 *
 * `missingReply` (Layer 8.4, ADR 0008) is true when the session's last
 * message is a user message with no follow-up at all — only reachable via
 * a server restart mid-turn, since a "stop" click always leaves its own
 * wrap-up assistant message (agent.Loop.stopped). Drives the "Antwort
 * fehlt — erneut senden?" button; `retryLastMessage` re-sends that same
 * user message's content as a new message rather than editing history
 * (spec 01-01: editing/deleting past messages is out of scope).
 *
 * `turnInProgress` (Layer 8.3) is true from the moment `send()`'s POST
 * succeeds until `chat.message`/`chat.turn_error` arrives — wider than
 * `sending` (which only covers the POST itself, near-instant per ADR
 * 0008's fire-and-forget shape) and doesn't have `sending`'s gap before
 * the first live SSE signal, unlike reconstructing it from
 * streamingText/toolProgress would. Used to disable "Neue Session" while
 * a turn is running. `hasPendingConfirmCards` is true when the current
 * session has a Confirm-Card no one has acted on yet — used to adjust
 * that same dialog's warning text (spec 01-01, ADR 0009).
 */
export function useChat(profileId: string) {
  const sessions = shallowRef<ChatSession[]>([]);
  const currentSessionId = shallowRef<string | null>(null);
  const messages = shallowRef<ChatMessage[]>([]);
  const loading = shallowRef(false);
  const sending = shallowRef(false);
  const error = shallowRef<string | null>(null);
  const streamingText = shallowRef('');
  const toolProgress = shallowRef<string | null>(null);
  const turnInProgress = shallowRef(false);

  let eventSource: EventSource | null = null;

  const activeSession = computed(() => sessions.value.find((session) => !session.ended_at));
  const canSend = computed(
    () => currentSessionId.value !== null && currentSessionId.value === activeSession.value?.id,
  );
  const hasPendingConfirmCards = computed(() => messages.value.some(hasPendingConfirmCard));
  const missingReply = computed(() => {
    const last = messages.value[messages.value.length - 1];
    return !turnInProgress.value && last?.role === 'user';
  });
  const displayMessages = computed<ChatMessage[]>(() => {
    const liveContent = streamingText.value || toolProgress.value;
    if (!liveContent || !currentSessionId.value) {
      return messages.value;
    }
    return [
      ...messages.value,
      {
        id: '__streaming__',
        session_id: currentSessionId.value,
        role: 'assistant',
        content: liveContent,
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

  /** Patches one tool_call's status in the local message cache in place
   * (Layer 8.3) — ChatConfirmCard.vue only keeps its own local `status`
   * ref for its own display, it has no way to update this composable's
   * `messages` on its own. Without this, `hasPendingConfirmCards` would
   * stay stale (still counting an already-resolved card as pending) until
   * the next full fetchMessages() call, which could be an entire turn
   * away or never if the session goes quiet. */
  function markToolCallResolved(
    messageId: string,
    callIndex: number,
    status: 'confirmed' | 'rejected',
  ) {
    messages.value = messages.value.map((message) => {
      if (message.id !== messageId || !message.tool_calls) {
        return message;
      }
      return {
        ...message,
        tool_calls: message.tool_calls.map((call, index) =>
          index === callIndex ? { ...call, status } : call,
        ),
      };
    });
  }

  async function selectSession(sessionId: string) {
    streamingText.value = '';
    toolProgress.value = null;
    turnInProgress.value = false;
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
      turnInProgress.value = true;
    } catch (sendError) {
      error.value = sendError instanceof Error ? sendError.message : String(sendError);
      throw sendError;
    } finally {
      sending.value = false;
    }
  }

  /** Spec 01-01 "stop" control: sets the backend turn's cancellation flag.
   * Doesn't clear turnInProgress itself — agent.Loop.stopped()'s wrap-up
   * roundtrip still fires chat.message like any other turn end, so the SSE
   * handler is the single place that flag ever clears, same as before. A
   * 404 (StopTurn's harmless race with the turn ending on its own) is
   * swallowed rather than surfaced as a user-facing error. */
  async function stop() {
    if (!currentSessionId.value) {
      return;
    }
    try {
      await chatApi.stopTurn(profileId, currentSessionId.value);
    } catch (stopError) {
      if (!(stopError instanceof ApiError) || stopError.status !== 404) {
        error.value = stopError instanceof Error ? stopError.message : String(stopError);
      }
    }
  }

  /** Re-sends the last (reply-less) user message's content as a new
   * message — the "Antwort fehlt — erneut senden?" button (missingReply). */
  async function retryLastMessage() {
    const last = messages.value[messages.value.length - 1];
    if (!last || last.role !== 'user') {
      return;
    }
    await send(last.content, last.attachment_path);
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
    toolProgress.value = null;
    turnInProgress.value = false;
  }

  /** Refetches sessions only, to pick up token_count_estimate after a turn
   * ends (the donut chart's data source) — separate from fetchMessages()
   * since fetchSessions() would also re-resolve currentSessionId, which
   * isn't needed here and messages already get their own refetch. */
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
        toolProgress.value = null;
        streamingText.value += payload.delta;
      }
    });
    eventSource.addEventListener('chat.tool_progress', (event) => {
      const payload = parseSSEPayload<ChatToolProgressPayload>(event);
      if (payload && payload.session_id === currentSessionId.value) {
        toolProgress.value = toolProgressLabels[payload.tool] ?? payload.tool;
      }
    });
    eventSource.addEventListener('chat.message', (event) => {
      const payload = parseSSEPayload<ChatMessagePayload>(event);
      if (payload && payload.session_id === currentSessionId.value) {
        streamingText.value = '';
        toolProgress.value = null;
        turnInProgress.value = false;
        void fetchMessages();
        void refreshTokenCount();
      }
    });
    eventSource.addEventListener('chat.turn_error', (event) => {
      const payload = parseSSEPayload<ChatTurnErrorPayload>(event);
      if (payload && payload.session_id === currentSessionId.value) {
        streamingText.value = '';
        toolProgress.value = null;
        turnInProgress.value = false;
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
    hasPendingConfirmCards,
    missingReply,
    messages: displayMessages,
    loading: readonly(loading),
    sending: readonly(sending),
    turnInProgress: readonly(turnInProgress),
    error: readonly(error),
    fetchSessions,
    selectSession,
    send,
    stop,
    retryLastMessage,
    startNewSession,
    markToolCallResolved,
  };
}
