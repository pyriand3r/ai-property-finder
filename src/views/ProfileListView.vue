<script setup lang="ts">
import { NAlert, NButton, NCollapseTransition, NH1, NSpin } from 'naive-ui';
import { useMessage } from 'naive-ui';
import { onMounted, shallowRef } from 'vue';
import { useRouter } from 'vue-router';

import ProfileCreateForm from '@/components/profile/ProfileCreateForm.vue';
import ProfileTable from '@/components/profile/ProfileTable.vue';
import { useProfiles } from '@/composables/useProfiles';

const router = useRouter();
const message = useMessage();
const {
  activeProfiles,
  archivedProfiles,
  loading,
  error,
  fetchProfiles,
  createProfile,
  renameProfile,
  archiveProfile,
  deleteProfile,
} = useProfiles();

const creating = shallowRef(false);

onMounted(fetchProfiles);

function openProfile(profileId: string) {
  router.push({ name: 'profile-shell', params: { profileId } });
}

async function handleCreate(name: string) {
  creating.value = true;
  try {
    await createProfile(name);
  } catch (createError) {
    message.error(createError instanceof Error ? createError.message : String(createError));
  } finally {
    creating.value = false;
  }
}

async function handleRename(profileId: string, name: string) {
  try {
    await renameProfile(profileId, name);
  } catch (renameError) {
    message.error(renameError instanceof Error ? renameError.message : String(renameError));
  }
}

async function handleArchive(profileId: string) {
  try {
    await archiveProfile(profileId);
  } catch (archiveError) {
    message.error(archiveError instanceof Error ? archiveError.message : String(archiveError));
  }
}

async function handleDelete(profileId: string) {
  try {
    await deleteProfile(profileId);
    message.success('Profil gelöscht');
  } catch (deleteError) {
    message.error(deleteError instanceof Error ? deleteError.message : String(deleteError));
  }
}
</script>

<template>
  <main class="profile-list-view">
    <div class="profile-list-header">
      <NH1>Property Finder</NH1>
      <NButton text @click="router.push({ name: 'global-context' })"> Globaler Kontext </NButton>
    </div>

    <ProfileCreateForm :creating="creating" @create="handleCreate" />

    <NAlert v-if="error" type="error" :title="error" class="profile-list-error" />

    <NSpin :show="loading">
      <section class="profile-list-section">
        <h2>Profile</h2>
        <ProfileTable
          :profiles="activeProfiles"
          empty-text="Noch keine Profile — leg oben ein neues an."
          @open="openProfile"
          @rename="handleRename"
          @archive="handleArchive"
          @delete="handleDelete"
        />
      </section>

      <NCollapseTransition :show="archivedProfiles.length > 0">
        <section class="profile-list-section">
          <h2>Archiv</h2>
          <ProfileTable
            :profiles="archivedProfiles"
            archived
            empty-text=""
            @open="openProfile"
            @rename="handleRename"
            @delete="handleDelete"
          />
        </section>
      </NCollapseTransition>
    </NSpin>
  </main>
</template>

<style scoped>
.profile-list-view {
  max-width: 720px;
  margin: 0 auto;
  padding: 2rem 1rem;
}

.profile-list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.profile-list-error {
  margin-block: 1rem;
}

.profile-list-section {
  margin-top: 2rem;
}
</style>
