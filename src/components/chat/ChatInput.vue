<script setup lang="ts">
import { NButton, NIcon, NInput, NTag, useMessage } from 'naive-ui';
import { shallowRef } from 'vue';

import { uploadImage, uploadPdf } from '@/api/upload';

const props = defineProps<{
  disabled: boolean;
  sending: boolean;
}>();

const emit = defineEmits<{
  send: [content: string, attachmentPath?: string];
}>();

const message = useMessage();
const content = shallowRef('');
const attachmentPath = shallowRef<string | undefined>(undefined);
const attachmentName = shallowRef<string | undefined>(undefined);
const uploading = shallowRef(false);
const dragOver = shallowRef(false);

function submit() {
  const trimmed = content.value.trim();
  if ((!trimmed && !attachmentPath.value) || props.disabled || props.sending) {
    return;
  }
  emit('send', trimmed, attachmentPath.value);
  content.value = '';
  attachmentPath.value = undefined;
  attachmentName.value = undefined;
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault();
    submit();
  }
}

async function handleFiles(files: FileList | null) {
  const file = files?.[0];
  if (!file) {
    return;
  }
  const isPdf = file.type === 'application/pdf';
  const isImage = file.type === 'image/jpeg' || file.type === 'image/png';
  if (!isPdf && !isImage) {
    message.error('Nur PDF-, JPG- oder PNG-Dateien werden unterstützt');
    return;
  }
  uploading.value = true;
  try {
    const result = isPdf ? await uploadPdf(file) : await uploadImage(file);
    attachmentPath.value = result.path;
    attachmentName.value = file.name;
  } catch (uploadError) {
    message.error(uploadError instanceof Error ? uploadError.message : String(uploadError));
  } finally {
    uploading.value = false;
  }
}

function handleDrop(event: DragEvent) {
  dragOver.value = false;
  void handleFiles(event.dataTransfer?.files ?? null);
}

function handleFileInput(event: Event) {
  void handleFiles((event.target as HTMLInputElement).files);
}
</script>

<template>
  <div
    class="chat-input"
    :class="{ 'chat-input--drag-over': dragOver }"
    @dragover.prevent="dragOver = true"
    @dragleave.prevent="dragOver = false"
    @drop.prevent="handleDrop"
  >
    <NTag
      v-if="attachmentName"
      closable
      class="chat-input-attachment"
      @close="
        attachmentPath = undefined;
        attachmentName = undefined;
      "
    >
      {{ attachmentName }}
    </NTag>

    <div class="chat-input-row">
      <label class="chat-input-file-button">
        <NIcon>📎</NIcon>
        <input
          type="file"
          accept="application/pdf,image/jpeg,image/png"
          hidden
          @change="handleFileInput"
        />
      </label>
      <NInput
        v-model:value="content"
        type="textarea"
        placeholder="Nachricht an den Agenten… (URL einfügen, PDF oder Bild ablegen)"
        :autosize="{ minRows: 1, maxRows: 6 }"
        :disabled="disabled"
        @keydown="handleKeydown"
      />
      <NButton
        type="primary"
        :loading="sending || uploading"
        :disabled="disabled || (!content.trim() && !attachmentPath)"
        @click="submit"
      >
        Senden
      </NButton>
    </div>
  </div>
</template>

<style scoped>
.chat-input {
  border-top: 1px solid var(--n-border-color, rgba(128, 128, 128, 0.3));
  padding: 0.75rem;
}

.chat-input--drag-over {
  background: rgba(24, 160, 88, 0.08);
}

.chat-input-attachment {
  margin-bottom: 0.5rem;
}

.chat-input-row {
  display: flex;
  align-items: flex-end;
  gap: 0.5rem;
}

.chat-input-file-button {
  cursor: pointer;
  padding: 0.25rem;
}
</style>
