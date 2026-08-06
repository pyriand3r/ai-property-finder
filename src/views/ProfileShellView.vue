<script setup lang="ts">
import { NButton, NH1, NTabPane, NTabs } from 'naive-ui';
import { defineAsyncComponent, onMounted, shallowRef } from 'vue';
import { useRouter } from 'vue-router';

import { getProfile } from '@/api/profiles';
import ChatView from '@/components/chat/ChatView.vue';
import ProfileContextPanel from '@/components/context/ProfileContextPanel.vue';
import CriteriaEditor from '@/components/criteria/CriteriaEditor.vue';
import ListingsPanel from '@/components/listings/ListingsPanel.vue';

// OpenLayers is a heavy dependency (~300kB) only the Karte tab needs — load
// it lazily so every other tab (Chat, Kriterien, Inserate) doesn't pay for
// it up front.
const ListingsMapPanel = defineAsyncComponent(
  () => import('@/components/listings/ListingsMapPanel.vue'),
);

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

    <NTabs type="line" animated default-value="chat">
      <NTabPane name="chat" tab="Chat">
        <ChatView :profile-id="profileId" />
      </NTabPane>
      <NTabPane name="context" tab="Context">
        <ProfileContextPanel :profile-id="profileId" />
      </NTabPane>
      <NTabPane name="criteria" tab="Kriterien">
        <CriteriaEditor :profile-id="profileId" />
      </NTabPane>
      <NTabPane name="listings" tab="Inserate">
        <ListingsPanel :profile-id="profileId" />
      </NTabPane>
      <NTabPane name="map" tab="Karte">
        <ListingsMapPanel :profile-id="profileId" />
      </NTabPane>
    </NTabs>
  </main>
</template>

<style scoped>
.profile-shell {
  padding: 2rem 2rem;
}
</style>
