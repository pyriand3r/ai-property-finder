<script setup lang="ts">
import { NButton, NCard, NSpace, NTag, NText } from 'naive-ui';
import { computed, shallowRef } from 'vue';

import { confirmToolCall, rejectToolCall } from '@/api/toolCalls';
import { decodeUnicodeEscapes } from '@/utils/chat';

import type { ProposeCriteriaArgs, UpdateContextArgs } from '@/types/chat';

/**
 * One pending propose_criteria/update_context proposal, rendered as a
 * diff-ish card with Übernehmen/Verwerfen (ADR 0009/0004, Layer 6.5).
 * Nothing was applied when the Agent called the tool — the arguments here
 * are the full proposal, already persisted verbatim in this message's
 * tool_calls. Clicking a button is the only thing that ever applies it.
 * A new criterion (action=add) is already stored as "proposed" (docs/adr/0024);
 * its card shows Freischalten, which activates that row, and Verwerfen deletes it.
 */
const props = defineProps<{
  messageId: string;
  callIndex: number;
  toolName: 'propose_criteria' | 'update_context';
  input: unknown;
  /** The call's persisted status (ADR 0017) — initializes the card's
   * displayed state so a reload/remount shows what actually happened
   * instead of resetting to the button pair. */
  status?: 'confirmed' | 'rejected';
}>();

const emit = defineEmits<{
  /** Fires right after a successful confirm/reject — this component only
   * keeps its own local `status` for its own display; without this event
   * the parent's message list (useChat.ts) has no way to know the call is
   * no longer pending, which matters for anything deriving state from it
   * (Layer 8.3's "Neue Session" warning checks for pending Confirm-Cards
   * across the whole session). */
  resolved: [status: 'confirmed' | 'rejected'];
}>();

type Status = 'pending' | 'confirmed' | 'rejected';

const status = shallowRef<Status>(props.status ?? 'pending');
const busy = shallowRef(false);
const error = shallowRef<string | null>(null);

const criteriaArgs = computed(() =>
  props.toolName === 'propose_criteria' ? (props.input as ProposeCriteriaArgs) : null,
);
const contextArgs = computed(() =>
  props.toolName === 'update_context' ? (props.input as UpdateContextArgs) : null,
);

const confirmLabel = computed(() =>
  criteriaArgs.value?.action === 'add' ? 'Freischalten' : 'Übernehmen',
);

const title = computed(() => {
  if (criteriaArgs.value) {
    return criteriaArgs.value.action === 'edit'
      ? 'Kriterium ändern?'
      : 'Neues Kriterium freischalten?';
  }
  return 'Kontext-Ergänzung vorschlagen?';
});

async function confirm() {
  busy.value = true;
  error.value = null;
  try {
    await confirmToolCall(props.messageId, props.callIndex);
    status.value = 'confirmed';
    emit('resolved', 'confirmed');
  } catch (confirmError) {
    error.value = confirmError instanceof Error ? confirmError.message : String(confirmError);
  } finally {
    busy.value = false;
  }
}

async function reject() {
  busy.value = true;
  error.value = null;
  try {
    await rejectToolCall(props.messageId, props.callIndex);
    status.value = 'rejected';
    emit('resolved', 'rejected');
  } catch (rejectError) {
    error.value = rejectError instanceof Error ? rejectError.message : String(rejectError);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <NCard size="small" class="chat-confirm-card" :title="title">
    <div v-if="criteriaArgs" class="chat-confirm-card-body">
      <NText strong>{{ decodeUnicodeEscapes(criteriaArgs.name) }}</NText>
      <NText v-if="criteriaArgs.description" depth="3">{{
        decodeUnicodeEscapes(criteriaArgs.description)
      }}</NText>
      <NSpace size="small">
        <NTag size="small">Gewicht {{ criteriaArgs.weight ?? 1 }}</NTag>
        <NTag size="small">{{ criteriaArgs.is_subjective ? 'Subjektiv' : 'Objektiv' }}</NTag>
      </NSpace>
    </div>
    <div v-else-if="contextArgs" class="chat-confirm-card-body">
      <NTag size="small">{{
        contextArgs.scope === 'global' ? 'Globaler Kontext' : 'Profil-Kontext'
      }}</NTag>
      <NText class="chat-confirm-card-content">{{
        decodeUnicodeEscapes(contextArgs.content)
      }}</NText>
    </div>

    <NText v-if="error" type="error" class="chat-confirm-card-error">{{ error }}</NText>

    <template #footer>
      <NSpace v-if="status === 'pending'">
        <NButton type="primary" size="small" :loading="busy" @click="confirm">{{
          confirmLabel
        }}</NButton>
        <NButton size="small" :disabled="busy" @click="reject">Verwerfen</NButton>
      </NSpace>
      <NTag v-else-if="status === 'confirmed'" type="success" size="small">{{
        criteriaArgs?.action === 'add' ? 'Freigeschaltet' : 'Übernommen'
      }}</NTag>
      <NTag v-else type="default" size="small">Verworfen</NTag>
    </template>
  </NCard>
</template>

<style scoped>
.chat-confirm-card {
  margin-top: 0.5rem;
  max-width: 360px;
}

.chat-confirm-card-body {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.chat-confirm-card-content {
  white-space: pre-wrap;
}

.chat-confirm-card-error {
  display: block;
  margin-top: 0.5rem;
}
</style>
