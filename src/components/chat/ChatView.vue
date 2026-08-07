<script setup lang="ts">
import { NAlert, NButton, NSelect, useDialog } from 'naive-ui';
import { computed, onMounted, shallowRef } from 'vue';

import ChatInput from '@/components/chat/ChatInput.vue';
import ChatMessageList from '@/components/chat/ChatMessageList.vue';
import TokenDonut from '@/components/chat/TokenDonut.vue';
import { useChat } from '@/composables/useChat';

const props = defineProps<{
  profileId: string;
}>();

const dialog = useDialog();
const {
  sessions,
  currentSessionId,
  activeSession,
  canSend,
  hasPendingConfirmCards,
  missingReply,
  messages,
  sending,
  turnInProgress,
  error,
  fetchSessions,
  selectSession,
  send,
  stop,
  retryLastMessage,
  startNewSession,
  markToolCallResolved,
} = useChat(props.profileId);

onMounted(fetchSessions);

const stopping = shallowRef(false);

async function handleStop() {
  stopping.value = true;
  try {
    await stop();
  } finally {
    stopping.value = false;
  }
}

function handleRetry() {
  void retryLastMessage();
}

const sessionOptions = computed(() =>
  sessions.value.map((session) => ({
    label:
      new Date(session.created_at).toLocaleString('de-DE', {
        dateStyle: 'medium',
        timeStyle: 'short',
      }) + (session.ended_at ? '' : ' (aktiv)'),
    value: session.id,
  })),
);

function handleSend(content: string, attachmentPath?: string) {
  void send(content, attachmentPath);
}

function handleCallResolved(
  messageId: string,
  callIndex: number,
  status: 'confirmed' | 'rejected',
) {
  markToolCallResolved(messageId, callIndex, status);
}

function handleNewSession() {
  // Layer 8.3: the button itself is disabled while turnInProgress, so this
  // only ever fires between turns — no separate "a turn is running" guard
  // needed here. The Confirm-Card warning is additive, not a replacement:
  // an open proposal isn't lost (it's still reachable/actionable via the
  // old session in the dropdown, ToolCallService doesn't gate on session
  // active-ness), just easy to forget about once a new session starts.
  const content = hasPendingConfirmCards.value
    ? 'Es gibt noch unbeantwortete Vorschläge (Confirm-Cards) im aktuellen Verlauf. Sie bleiben ' +
      'über die alte Session im Dropdown erreichbar, werden aber nicht automatisch in die neue ' +
      'Session übernommen. Der bisherige Verlauf bleibt erhalten, die KI startet mit leerem Kontext.'
    : 'Der bisherige Verlauf bleibt als Chat-Verlauf erhalten, aber die KI startet mit leerem Kontext.';
  dialog.warning({
    title: 'Neue Session starten?',
    content,
    positiveText: 'Neue Session starten',
    negativeText: 'Abbrechen',
    onPositiveClick: () => void startNewSession(),
  });
}
</script>

<template>
  <div class="chat-view">
    <div class="chat-view-header">
      <NSelect
        :value="currentSessionId"
        :options="sessionOptions"
        class="chat-view-session-select"
        @update:value="selectSession"
      />
      <TokenDonut :token-count-estimate="activeSession?.token_count_estimate" />
      <NButton v-if="turnInProgress" :loading="stopping" :disabled="stopping" @click="handleStop">
        Stop
      </NButton>
      <NButton :disabled="turnInProgress" @click="handleNewSession">Neue Session</NButton>
    </div>

    <NAlert v-if="error" type="error" :title="error" class="chat-view-error" />

    <ChatMessageList
      :messages="messages"
      :missing-reply="missingReply"
      @call-resolved="handleCallResolved"
      @retry="handleRetry"
    />

    <ChatInput :disabled="!canSend" :sending="sending" @send="handleSend" />
  </div>
</template>

<style scoped>
.chat-view {
  display: flex;
  flex-direction: column;
  height: 70vh;
}

.chat-view-header {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding-bottom: 0.75rem;
}

.chat-view-session-select {
  flex-grow: 1;
}

.chat-view-error {
  margin-bottom: 0.5rem;
}
</style>
