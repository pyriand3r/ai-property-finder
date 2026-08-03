<script setup lang="ts">
import { NEmpty, NText } from 'naive-ui';
import { nextTick, useTemplateRef, watch } from 'vue';

import type { ChatMessage } from '@/types/chat';

const props = defineProps<{
  messages: readonly ChatMessage[];
}>();

const listRef = useTemplateRef<HTMLDivElement>('list');

watch(
  () => props.messages.length,
  async () => {
    await nextTick();
    listRef.value?.scrollTo({ top: listRef.value.scrollHeight, behavior: 'smooth' });
  },
);

function formatTime(iso: string) {
  return new Date(iso).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' });
}
</script>

<template>
  <div ref="list" class="chat-message-list">
    <NEmpty v-if="messages.length === 0" description="Noch keine Nachrichten" />
    <div
      v-for="message in messages"
      :key="message.id"
      class="chat-message"
      :class="`chat-message--${message.role}`"
    >
      <div class="chat-message-bubble">
        <p class="chat-message-content">{{ message.content }}</p>
        <NText depth="3" class="chat-message-time">{{ formatTime(message.created_at) }}</NText>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat-message-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  overflow-y: auto;
  padding: 1rem;
  flex-grow: 1;
}

.chat-message {
  display: flex;
}

.chat-message--user {
  justify-content: flex-end;
}

.chat-message--assistant,
.chat-message--tool {
  justify-content: flex-start;
}

.chat-message-bubble {
  max-width: 70%;
  padding: 0.5rem 0.75rem;
  border-radius: 0.5rem;
  background: var(--n-color-target, rgba(128, 128, 128, 0.15));
}

.chat-message-content {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
}

.chat-message-time {
  font-size: 0.7rem;
}
</style>
