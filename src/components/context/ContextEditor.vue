<script setup lang="ts">
import DOMPurify from 'dompurify';
import { marked } from 'marked';
import {
  NAlert,
  NButton,
  NEmpty,
  NInput,
  NList,
  NListItem,
  NSpace,
  NSpin,
  NTag,
  useDialog,
} from 'naive-ui';
import { computed, ref, watch } from 'vue';

import type { ContextRevision } from '@/types/context';

const props = defineProps<{
  content: string;
  history: readonly ContextRevision[];
  loading: boolean;
  saving: boolean;
  error: string | null;
}>();

const emit = defineEmits<{
  save: [contentMd: string];
  restore: [revisionId: string];
}>();

const dialog = useDialog();

const editing = ref(false);
const draft = ref(props.content);

// content arrives asynchronously (parent fetches after mount) — keep the
// draft in sync until the user starts editing, same pattern as
// ListingForm.vue's draft/listing sync.
watch(
  () => props.content,
  (updated) => {
    if (!editing.value) {
      draft.value = updated;
    }
  },
);

const renderedHtml = computed(() =>
  props.content ? DOMPurify.sanitize(marked.parse(props.content, { async: false })) : '',
);

function startEditing() {
  draft.value = props.content;
  editing.value = true;
}

function cancel() {
  draft.value = props.content;
  editing.value = false;
}

function save() {
  editing.value = false;
  if (draft.value !== props.content) {
    emit('save', draft.value);
  }
}

function confirmRestore(revision: ContextRevision) {
  dialog.warning({
    title: 'Revision wiederherstellen?',
    content:
      'Der aktuelle Text wird durch diese ältere Version ersetzt. Der jetzige Stand bleibt selbst als neue Verlaufs-Revision erhalten.',
    positiveText: 'Wiederherstellen',
    negativeText: 'Abbrechen',
    onPositiveClick: () => emit('restore', revision.id),
  });
}
</script>

<template>
  <div class="context-editor">
    <NAlert v-if="error" type="error" :title="error" class="context-editor-error" />

    <NSpin :show="loading || saving">
      <div class="context-editor-toolbar">
        <NButton v-if="!editing" size="small" @click="startEditing">Bearbeiten</NButton>
        <NSpace v-else size="small">
          <NButton size="small" type="primary" @click="save">Speichern</NButton>
          <NButton size="small" @click="cancel">Abbrechen</NButton>
        </NSpace>
      </div>

      <NInput
        v-if="editing"
        v-model:value="draft"
        type="textarea"
        :autosize="{ minRows: 10, maxRows: 40 }"
        placeholder="Markdown …"
      />
      <div v-else-if="content" class="context-editor-rendered" v-html="renderedHtml" />
      <NEmpty v-else description="Noch kein Context-Text vorhanden" />

      <section class="context-editor-history">
        <h3>Verlauf</h3>
        <NEmpty v-if="history.length === 0" description="Noch keine Änderungen" />
        <NList v-else bordered>
          <NListItem v-for="revision in history" :key="revision.id">
            <div class="context-editor-history-row">
              <NTag size="small" :type="revision.changed_by === 'agent' ? 'info' : 'default'">
                {{ revision.changed_by === 'agent' ? 'Agent' : 'Nutzer' }}
              </NTag>
              <span class="context-editor-history-date">
                {{ new Date(revision.created_at).toLocaleString('de-DE') }}
              </span>
              <NButton size="tiny" quaternary @click="confirmRestore(revision)">
                Wiederherstellen
              </NButton>
            </div>
          </NListItem>
        </NList>
      </section>
    </NSpin>
  </div>
</template>

<style scoped>
.context-editor-error {
  margin-bottom: 1rem;
}

.context-editor-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 0.5rem;
}

.context-editor-rendered {
  line-height: 1.5;
}

.context-editor-history {
  margin-top: 2rem;
}

.context-editor-history-row {
  display: flex;
  align-items: center;
  gap: 1rem;
  width: 100%;
}

.context-editor-history-date {
  flex-grow: 1;
  font-size: 0.85rem;
  opacity: 0.75;
}
</style>
