<script setup lang="ts">
import { NEmpty, NText } from 'naive-ui';
import { computed, nextTick, useTemplateRef, watch } from 'vue';

import ChatConfirmCard from '@/components/chat/ChatConfirmCard.vue';
import ChatListingCard from '@/components/chat/ChatListingCard.vue';

import type { ChatMessage, StoredToolCall } from '@/types/chat';

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

const CONFIRMABLE_TOOLS = new Set(['propose_criteria', 'update_context']);

/** Confirm-Card-worthy tool calls, paired with their index in tool_calls
 * — the confirm/reject endpoint addresses a call by that array index, so
 * it must survive filtering (spec 00-03/ADR 0009, Layer 6.5). */
function confirmableCalls(message: ChatMessage): { call: StoredToolCall; index: number }[] {
  return (message.tool_calls ?? [])
    .map((call, index) => ({ call, index }))
    .filter(({ call }) => CONFIRMABLE_TOOLS.has(call.name));
}

/** Every roundtrip's assistant message is persisted (Layer 6.5), not just
 * the turn's final reply — an intermediate "calling fetch_url/save_listing"
 * roundtrip has empty content and nothing Confirm-Card-worthy, so it'd
 * render as a blank bubble. Hide those; the chat.tool_progress event
 * already gave the user live feedback for them. */
const visibleMessages = computed(() =>
  props.messages.filter(
    (message) =>
      message.content.trim() !== '' || confirmableCalls(message).length > 0 || !!message.listing_id,
  ),
);
</script>

<template>
  <div ref="list" class="chat-message-list">
    <NEmpty v-if="visibleMessages.length === 0" description="Noch keine Nachrichten" />
    <div
      v-for="message in visibleMessages"
      :key="message.id"
      class="chat-message"
      :class="`chat-message--${message.role}`"
    >
      <div class="chat-message-bubble">
        <p v-if="message.content" class="chat-message-content">{{ message.content }}</p>
        <NText depth="3" class="chat-message-time">{{ formatTime(message.created_at) }}</NText>
        <ChatListingCard v-if="message.listing_id" :listing-id="message.listing_id" />
        <ChatConfirmCard
          v-for="{ call, index } in confirmableCalls(message)"
          :key="call.id"
          :message-id="message.id"
          :call-index="index"
          :tool-name="call.name as 'propose_criteria' | 'update_context'"
          :input="call.input"
          :status="call.status"
        />
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
