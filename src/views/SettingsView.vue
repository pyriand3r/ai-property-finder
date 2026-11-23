<script setup lang="ts">
import { NAlert, NButton, NH1, NSpin, useMessage } from 'naive-ui';
import { onMounted } from 'vue';
import { useRouter } from 'vue-router';

import LlmConfigForm from '@/components/settings/LlmConfigForm.vue';
import { useLlmConfig } from '@/composables/useLlmConfig';

import type { LlmConfigUpdate } from '@/types/config';

const router = useRouter();
const message = useMessage();
const { config, loading, saving, error, fetchConfig, save } = useLlmConfig();

onMounted(fetchConfig);

async function handleSave(update: LlmConfigUpdate) {
  try {
    await save(update);
    message.success(
      'Einstellungen gespeichert – gilt ab dem nächsten Chat-Turn bzw. der nächsten Bewertung',
    );
  } catch (saveError) {
    message.error(saveError instanceof Error ? saveError.message : String(saveError));
  }
}
</script>

<template>
  <main class="settings-view">
    <NButton text @click="router.push({ name: 'profile-list' })"> &larr; Alle Profile </NButton>
    <NH1>Einstellungen</NH1>

    <NAlert v-if="error" type="error" :title="error" class="settings-error" />

    <NSpin :show="loading">
      <section class="settings-section">
        <h2>LLM</h2>
        <p class="settings-hint">
          Änderungen wirken sofort, ohne Neustart. Ein laufender Agent-Turn nutzt noch die alten
          Werte. Der API-Key wird nie angezeigt.
        </p>
        <LlmConfigForm v-if="config" :config="config" :saving="saving" @save="handleSave" />
      </section>
    </NSpin>
  </main>
</template>

<style scoped>
.settings-view {
  max-width: 720px;
  margin: 0 auto;
  padding: 2rem 1rem;
}

.settings-error {
  margin-block: 1rem;
}

.settings-section {
  margin-top: 2rem;
}

.settings-hint {
  opacity: 0.7;
}
</style>
