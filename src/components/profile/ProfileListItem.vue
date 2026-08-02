<script setup lang="ts">
import { NButton, NInput, NPopconfirm, NSpace, NThing } from 'naive-ui';
import { shallowRef } from 'vue';

import type { Profile } from '@/types/profile';

const props = defineProps<{
  profile: Profile;
  /** Hides the archive action for profiles already archived (Layer 4.2 has
   * no "unarchive" endpoint — see AGENTS.md/implementation-plan.md). */
  archived?: boolean;
}>();

const emit = defineEmits<{
  open: [profileId: string];
  rename: [profileId: string, name: string];
  archive: [profileId: string];
  delete: [profileId: string];
}>();

const editing = shallowRef(false);
const draftName = shallowRef(props.profile.name);

function startRename() {
  draftName.value = props.profile.name;
  editing.value = true;
}

function confirmRename() {
  const trimmed = draftName.value.trim();
  editing.value = false;
  if (trimmed && trimmed !== props.profile.name) {
    emit('rename', props.profile.id, trimmed);
  }
}

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('de-DE', { dateStyle: 'medium' });
}
</script>

<template>
  <div class="profile-list-item">
    <NThing v-if="!editing" @click="emit('open', profile.id)">
      <template #header>
        <span class="profile-list-item-name">{{ profile.name }}</span>
      </template>
      <template #description> Angelegt am {{ formatDate(profile.created_at) }} </template>
    </NThing>
    <NInput
      v-else
      v-model:value="draftName"
      autofocus
      @keyup.enter="confirmRename"
      @blur="confirmRename"
    />

    <NSpace class="profile-list-item-actions">
      <NButton size="small" quaternary @click.stop="startRename">Umbenennen</NButton>
      <NButton v-if="!archived" size="small" quaternary @click.stop="emit('archive', profile.id)">
        Archivieren
      </NButton>
      <NPopconfirm @positive-click="emit('delete', profile.id)">
        <template #trigger>
          <NButton size="small" quaternary type="error" @click.stop> Löschen </NButton>
        </template>
        Profil "{{ profile.name }}" wirklich löschen? Alle Inserate, Chat-Verlauf und Context gehen
        dabei unwiderruflich verloren.
      </NPopconfirm>
    </NSpace>
  </div>
</template>

<style scoped>
.profile-list-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.75rem 0;
}

.profile-list-item-name {
  cursor: pointer;
}

.profile-list-item-actions {
  flex-shrink: 0;
}
</style>
