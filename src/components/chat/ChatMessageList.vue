<script setup lang="ts">
import { NButton, NEmpty, NText } from 'naive-ui';
import { computed, nextTick, useTemplateRef, watch } from 'vue';

import ChatConfirmCard from '@/components/chat/ChatConfirmCard.vue';
import ChatListingCard from '@/components/chat/ChatListingCard.vue';
import { confirmableCalls } from '@/utils/chat';

import type { ChatMessage } from '@/types/chat';

const props = defineProps<{
  messages: readonly ChatMessage[];
  /** True when the last message is a user message with no follow-up at all
   * (useChat.ts missingReply, Layer 8.4/ADR 0008) — renders the "Antwort
   * fehlt — erneut senden?" button below it. */
  missingReply?: boolean;
  /** True while a turn is running but no live content has shown yet
   * (useChat.ts awaitingSignal, RF.5) — renders a pulsing placeholder bubble
   * in the position the next Agent message will appear. */
  awaitingSignal?: boolean;
}>();

const emit = defineEmits<{
  /** Bubbles ChatConfirmCard's resolved event up with enough identity
   * (message id + tool_calls index) for the parent to patch its own
   * message cache — see useChat.ts markToolCallResolved (Layer 8.3). */
  callResolved: [messageId: string, callIndex: number, status: 'confirmed' | 'rejected'];
  /** "Antwort fehlt" button clicked — parent calls useChat.ts
   * retryLastMessage (Layer 8.4). */
  retry: [];
}>();

const listRef = useTemplateRef<HTMLDivElement>('list');

watch(
  () => [props.messages.length, props.awaitingSignal],
  async () => {
    await nextTick();
    listRef.value?.scrollTo({ top: listRef.value.scrollHeight, behavior: 'smooth' });
  },
);

function formatTime(iso: string) {
  return new Date(iso).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' });
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
          @resolved="(status) => emit('callResolved', message.id, index, status)"
        />
      </div>
    </div>
    <div v-if="awaitingSignal" class="chat-message chat-message--assistant">
      <div class="chat-message-bubble chat-message-bubble--awaiting">
        <span class="chat-typing-dot" />
        <span class="chat-typing-dot" />
        <span class="chat-typing-dot" />
      </div>
    </div>
    <div v-if="missingReply" class="chat-message-missing-reply">
      <NButton size="small" @click="emit('retry')">Antwort fehlt — erneut senden?</NButton>
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

.chat-message-missing-reply {
  display: flex;
  justify-content: flex-end;
  padding: 0 0.5rem;
}

.chat-message-bubble--awaiting {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.chat-typing-dot {
  width: 0.4rem;
  height: 0.4rem;
  border-radius: 50%;
  background: currentcolor;
  opacity: 0.4;
  animation: chat-typing-pulse 1.2s ease-in-out infinite;
}

.chat-typing-dot:nth-child(2) {
  animation-delay: 0.2s;
}

.chat-typing-dot:nth-child(3) {
  animation-delay: 0.4s;
}

@keyframes chat-typing-pulse {
  0%,
  60%,
  100% {
    opacity: 0.4;
    transform: scale(1);
  }
  30% {
    opacity: 1;
    transform: scale(1.3);
  }
}
</style>
