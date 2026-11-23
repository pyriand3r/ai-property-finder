<script setup lang="ts">
import { NButton, NH1, NTabPane, NTabs } from 'naive-ui';
import { computed, defineAsyncComponent, onMounted, shallowRef } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { getProfile } from '@/api/profiles';
import ChatView from '@/components/chat/ChatView.vue';
import ProfileContextPanel from '@/components/context/ProfileContextPanel.vue';
import CriteriaEditor from '@/components/criteria/CriteriaEditor.vue';
import ListingsPanel from '@/components/listings/ListingsPanel.vue';
import { useRejectionAnalysisToast } from '@/composables/useRejectionAnalysisToast';

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

/**
 * Tab names, matching the `NTabPane name` values below. The active tab lives in the
 * optional `:tab` route segment (`/profiles/:profileId/:tab?`, see `src/router/index.ts`)
 * so reload and deep links keep the tab. Missing or unknown values fall back to `chat`.
 */
const TABS = ['chat', 'context', 'criteria', 'listings', 'map'] as const;
type Tab = (typeof TABS)[number];

const router = useRouter();
const route = useRoute();

const activeTab = computed<Tab>(() => {
  const tab = route.params.tab;
  return TABS.find((name) => name === tab) ?? 'chat';
});

/** `replace`, not `push`: switching tabs must not fill the browser history. */
function handleTabChange(tab: string) {
  router.replace({ name: 'profile-shell', params: { profileId: props.profileId, tab } });
}
const profile = shallowRef<Profile | null>(null);

useRejectionAnalysisToast(props.profileId);

onMounted(async () => {
  profile.value = await getProfile(props.profileId);
});
</script>

<template>
  <main class="profile-shell">
    <NButton text @click="router.push({ name: 'profile-list' })"> &larr; Alle Profile </NButton>

    <NH1 v-if="profile">{{ profile.name }}</NH1>

    <NTabs type="line" animated :value="activeTab" @update:value="handleTabChange">
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
