<script setup lang="ts">
import { NAlert, NButton, NCard, NEmpty, NInput, NSpin, useDialog } from 'naive-ui';
import { onMounted, shallowRef } from 'vue';

import { useListingNotes } from '@/composables/useListingNotes';

import type { ListingNote } from '@/types/listing';

const props = defineProps<{
  listingId: string;
}>();

const dialog = useDialog();

const { notes, loading, error, fetchNotes, addNote, editNote, removeNote } = useListingNotes(
  props.listingId,
);

onMounted(fetchNotes);

const creating = shallowRef(false);
const newDraft = shallowRef('');

function startCreate() {
  newDraft.value = '';
  creating.value = true;
}

async function saveCreate() {
  if (!newDraft.value.trim()) {
    creating.value = false;
    return;
  }
  await addNote(newDraft.value);
  creating.value = false;
}

function cancelCreate() {
  creating.value = false;
}

const editingId = shallowRef<string | null>(null);
const editDraft = shallowRef('');

function startEdit(note: ListingNote) {
  editingId.value = note.id;
  editDraft.value = note.content;
}

async function saveEdit(noteId: string) {
  if (editDraft.value.trim()) {
    await editNote(noteId, editDraft.value);
  }
  editingId.value = null;
}

function cancelEdit() {
  editingId.value = null;
}

function confirmDelete(note: ListingNote) {
  dialog.warning({
    title: 'Notiz löschen?',
    content: 'Die Notiz wird unwiderruflich entfernt.',
    positiveText: 'Löschen',
    negativeText: 'Abbrechen',
    onPositiveClick: () => removeNote(note.id),
  });
}
</script>

<template>
  <div class="listing-notes">
    <NAlert v-if="error" type="error" :title="error" class="listing-notes-error" />

    <div class="listing-notes-header">
      <h3>Notizen</h3>
      <NButton size="small" quaternary :disabled="creating" @click="startCreate">
        + Neue Notiz
      </NButton>
    </div>

    <NSpin :show="loading">
      <NCard v-if="creating" size="small" class="listing-notes-card">
        <NInput
          v-model:value="newDraft"
          type="textarea"
          placeholder="Notiz eingeben..."
          :autosize="{ minRows: 2, maxRows: 10 }"
          autofocus
        />
        <template #footer>
          <NButton size="tiny" type="primary" @click="saveCreate">Speichern</NButton>
          <NButton size="tiny" quaternary @click="cancelCreate">Abbrechen</NButton>
        </template>
      </NCard>

      <NEmpty v-if="!creating && notes.length === 0" description="Noch keine Notizen vorhanden" />

      <NCard v-for="note in notes" :key="note.id" size="small" class="listing-notes-card">
        <template v-if="editingId === note.id">
          <NInput
            v-model:value="editDraft"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 10 }"
            autofocus
          />
        </template>
        <template v-else>
          <div class="listing-notes-actions">
            <NButton size="tiny" circle quaternary title="Bearbeiten" @click="startEdit(note)">
              ✎
            </NButton>
            <NButton size="tiny" circle quaternary title="Löschen" @click="confirmDelete(note)">
              🗑
            </NButton>
          </div>
          <p class="listing-notes-content">{{ note.content }}</p>
          <span class="listing-notes-timestamp">
            {{ new Date(note.updated_at).toLocaleString('de-DE') }}
          </span>
        </template>

        <template v-if="editingId === note.id" #footer>
          <NButton size="tiny" type="primary" @click="saveEdit(note.id)">Speichern</NButton>
          <NButton size="tiny" quaternary @click="cancelEdit">Abbrechen</NButton>
        </template>
      </NCard>
    </NSpin>
  </div>
</template>

<style scoped>
.listing-notes-error {
  margin-bottom: 1rem;
}

.listing-notes-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.listing-notes-card {
  position: relative;
  margin-top: 0.75rem;
}

.listing-notes-actions {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  display: flex;
  gap: 0.25rem;
}

.listing-notes-content {
  white-space: pre-wrap;
  margin: 0 2.5rem 0.5rem 0;
}

.listing-notes-timestamp {
  font-size: 0.8rem;
  color: var(--n-text-color-3, #888);
}
</style>
