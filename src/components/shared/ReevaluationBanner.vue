<script setup lang="ts">
import { NAlert, NButton, NProgress } from 'naive-ui';
import { computed } from 'vue';

import { useReevaluationBanner } from '@/composables/useReevaluationBanner';

const props = defineProps<{
  profileId: string;
}>();

const { staleCount, running, progress, error, triggerReevaluateAll } = useReevaluationBanner(
  props.profileId,
);

const progressPercent = computed(() => {
  if (!progress.value || progress.value.total === 0) {
    return 0;
  }
  return Math.round((progress.value.done / progress.value.total) * 100);
});
</script>

<template>
  <NAlert v-if="error" type="error" :title="error" class="reevaluation-banner" />
  <NAlert v-else-if="running" type="info" title="Neubewertung läuft…" class="reevaluation-banner">
    <NProgress
      v-if="progress"
      type="line"
      :percentage="progressPercent"
      :status="progress.failedCount > 0 ? 'warning' : 'default'"
    />
    <template v-if="progress">
      {{ progress.done }} von {{ progress.total }} erledigt
      <template v-if="progress.failedCount > 0"> — {{ progress.failedCount }} Fehler </template>
    </template>
  </NAlert>
  <NAlert v-else-if="staleCount > 0" type="warning" class="reevaluation-banner">
    <template #header>
      Bewertung veraltet — Kriterien wurden geändert. {{ staleCount }} Inserat{{
        staleCount === 1 ? '' : 'e'
      }}
      neu bewerten?
    </template>
    <NButton size="small" type="warning" @click="triggerReevaluateAll"> Alle neu bewerten </NButton>
  </NAlert>
</template>

<style scoped>
.reevaluation-banner {
  margin-bottom: 1rem;
}
</style>
