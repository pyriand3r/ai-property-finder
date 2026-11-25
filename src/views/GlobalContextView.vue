<script setup lang="ts">
import { NButton, NH1 } from 'naive-ui';
import { onMounted } from 'vue';
import { useRouter } from 'vue-router';

import {
  getGlobalContext,
  getGlobalContextHistory,
  restoreGlobalContext,
  updateGlobalContext,
} from '@/api/context';
import ContextEditor from '@/components/context/ContextEditor.vue';
import HazardSourcesPanel from '@/components/hazard/HazardSourcesPanel.vue';
import { useContextEditor } from '@/composables/useContextEditor';

const router = useRouter();

const { context, history, loading, saving, error, fetchAll, save, restore } = useContextEditor(
  getGlobalContext,
  updateGlobalContext,
  getGlobalContextHistory,
  restoreGlobalContext,
);

onMounted(fetchAll);
</script>

<template>
  <main class="global-context-view">
    <NButton text @click="router.push({ name: 'profile-list' })"> &larr; Alle Profile </NButton>
    <NH1>Globaler Kontext</NH1>

    <ContextEditor
      :content="context?.content_md ?? ''"
      :history="history"
      :loading="loading"
      :saving="saving"
      :error="error"
      @save="save"
      @restore="restore"
    />

    <HazardSourcesPanel />
  </main>
</template>

<style scoped>
.global-context-view {
  padding: 2rem 2rem;
}
</style>
