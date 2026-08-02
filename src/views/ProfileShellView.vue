<script setup lang="ts">
import { NButton, NEmpty, NH1, NTabPane, NTabs } from 'naive-ui';
import { onMounted, shallowRef } from 'vue';
import { useRouter } from 'vue-router';

import { getProfile } from '@/api/profiles';
import CriteriaEditor from '@/components/criteria/CriteriaEditor.vue';

import type { Profile } from '@/types/profile';

const props = defineProps<{
  profileId: string;
}>();

const router = useRouter();
const profile = shallowRef<Profile | null>(null);

onMounted(async () => {
  profile.value = await getProfile(props.profileId);
});
</script>

<template>
  <main class="profile-shell">
    <NButton text @click="router.push({ name: 'profile-list' })"> &larr; Alle Profile </NButton>

    <NH1 v-if="profile">{{ profile.name }}</NH1>

    <NTabs type="line" animated>
      <NTabPane name="chat" tab="Chat">
        <NEmpty description="Chat kommt in Layer 6" />
      </NTabPane>
      <NTabPane name="context" tab="Context">
        <NEmpty description="Context-Editor kommt in Layer 7" />
      </NTabPane>
      <NTabPane name="criteria" tab="Kriterien">
        <CriteriaEditor :profile-id="profileId" />
      </NTabPane>
      <NTabPane name="listings" tab="Inserate">
        <NEmpty description="Listen-/Karten-Ansicht kommt in Layer 5" />
      </NTabPane>
    </NTabs>
  </main>
</template>

<style scoped>
.profile-shell {
  max-width: 960px;
  margin: 0 auto;
  padding: 2rem 1rem;
}
</style>
