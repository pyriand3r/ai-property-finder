<script setup lang="ts">
import { NEmpty, NList, NListItem } from 'naive-ui';

import ProfileListItem from '@/components/profile/ProfileListItem.vue';

import type { Profile } from '@/types/profile';

defineProps<{
  profiles: readonly Profile[];
  archived?: boolean;
  emptyText: string;
}>();

const emit = defineEmits<{
  open: [profileId: string];
  rename: [profileId: string, name: string];
  archive: [profileId: string];
  delete: [profileId: string];
}>();
</script>

<template>
  <NEmpty v-if="profiles.length === 0" :description="emptyText" />
  <NList v-else bordered>
    <NListItem v-for="profile in profiles" :key="profile.id">
      <ProfileListItem
        :profile="profile"
        :archived="archived"
        @open="emit('open', $event)"
        @rename="(profileId, name) => emit('rename', profileId, name)"
        @archive="emit('archive', $event)"
        @delete="emit('delete', $event)"
      />
    </NListItem>
  </NList>
</template>
