import { computed, readonly, shallowRef } from 'vue';

import * as profilesApi from '@/api/profiles';

import type { Profile } from '@/types/profile';

/** Loads the profile list once and exposes create/rename/archive/delete
 * actions that keep the local list in sync with the backend. */
export function useProfiles() {
  const profiles = shallowRef<Profile[]>([]);
  const loading = shallowRef(false);
  const error = shallowRef<string | null>(null);

  const activeProfiles = computed(() => profiles.value.filter((profile) => !profile.archived_at));
  const archivedProfiles = computed(() => profiles.value.filter((profile) => profile.archived_at));

  async function fetchProfiles() {
    loading.value = true;
    error.value = null;
    try {
      profiles.value = await profilesApi.listProfiles();
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    } finally {
      loading.value = false;
    }
  }

  async function createProfile(name: string) {
    const created = await profilesApi.createProfile(name);
    profiles.value = [created, ...profiles.value];
    return created;
  }

  async function renameProfile(profileId: string, name: string) {
    const updated = await profilesApi.renameProfile(profileId, name);
    replaceProfile(updated);
  }

  async function archiveProfile(profileId: string) {
    const updated = await profilesApi.archiveProfile(profileId);
    replaceProfile(updated);
  }

  async function deleteProfile(profileId: string) {
    await profilesApi.deleteProfile(profileId);
    profiles.value = profiles.value.filter((profile) => profile.id !== profileId);
  }

  function replaceProfile(updated: Profile) {
    profiles.value = profiles.value.map((profile) =>
      profile.id === updated.id ? updated : profile,
    );
  }

  return {
    profiles: readonly(profiles),
    activeProfiles,
    archivedProfiles,
    loading: readonly(loading),
    error: readonly(error),
    fetchProfiles,
    createProfile,
    renameProfile,
    archiveProfile,
    deleteProfile,
  };
}
