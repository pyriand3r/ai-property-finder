<script setup lang="ts">
import { NAlert, NButton, NSelect, useDialog } from 'naive-ui';
import { computed, onMounted } from 'vue';

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
  messages,
  sending,
  error,
  fetchSessions,
  selectSession,
  send,
  startNewSession,
} = useChat(props.profileId);

onMounted(fetchSessions);

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

function handleNewSession() {
  // Layer 6.1: simple confirmation only — the "an open Confirm-Card would
  // be lost" warning and in-flight-turn disable come with Layer 8.3, once
  // Confirm-Cards and a real in-flight Agent turn exist to warn about.
  dialog.warning({
    title: 'Neue Session starten?',
    content:
      'Der bisherige Verlauf bleibt als Chat-Verlauf erhalten, aber die KI startet mit leerem Kontext.',
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
      <NButton @click="handleNewSession">Neue Session</NButton>
    </div>

    <NAlert v-if="error" type="error" :title="error" class="chat-view-error" />

    <ChatMessageList :messages="messages" />

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
