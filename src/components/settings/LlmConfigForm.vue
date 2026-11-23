<script setup lang="ts">
import { NButton, NForm, NFormItem, NInput, NInputNumber, NSelect } from 'naive-ui';
import { computed, shallowReactive, shallowRef, watch } from 'vue';

import type { AppConfig, LlmConfigUpdate } from '@/types/config';
import type { DeepReadonly } from 'vue';

const props = defineProps<{
  /** Read-only view: useLlmConfig() exposes its state through readonly(). */
  config: DeepReadonly<AppConfig>;
  /** True while the parent's save request is in flight. */
  saving: boolean;
}>();

const emit = defineEmits<{
  save: [update: LlmConfigUpdate];
}>();

/** Local draft of the editable fields. The API key is write-only: the draft starts empty and
 * is only sent when the user types a new one. */
const draft = shallowReactive({
  provider: '',
  model: '',
  base_url: '',
  timeout_seconds: 30,
  max_retries: 2,
  max_tool_calls_per_turn: 15,
  max_llm_roundtrips: 5,
  reasoning_language: 'de',
});
const apiKey = shallowRef('');

watch(
  () => props.config,
  (config) => {
    Object.assign(draft, {
      provider: config.llm.provider,
      model: config.llm.model,
      base_url: config.llm.base_url,
      timeout_seconds: config.llm.timeout_seconds,
      max_retries: config.llm.max_retries,
      max_tool_calls_per_turn: config.llm.max_tool_calls_per_turn,
      max_llm_roundtrips: config.llm.max_llm_roundtrips,
      reasoning_language: config.llm.reasoning_language,
    });
    apiKey.value = '';
  },
  { immediate: true },
);

const providerOptions = computed(() =>
  props.config.providers.map((provider) => ({ label: provider.label, value: provider.id })),
);

/** Model suggestions of the selected provider; the stored model is always selectable, even if
 * the backend list no longer names it. */
const modelOptions = computed(() => {
  const suggestions =
    props.config.providers.find((provider) => provider.id === draft.provider)?.models ?? [];
  const models =
    suggestions.includes(draft.model) || !draft.model ? suggestions : [draft.model, ...suggestions];
  return models.map((model) => ({ label: model, value: model }));
});

const apiKeyPlaceholder = computed(() =>
  props.config.llm.api_key_configured
    ? 'Gespeichert – leer lassen, um den Key zu behalten'
    : 'Noch kein API-Key gesetzt',
);

const canSave = computed(() => draft.provider !== '' && draft.model.trim() !== '');

function submit() {
  if (!canSave.value || props.saving) {
    return;
  }
  const update: LlmConfigUpdate = { ...draft };
  const trimmedKey = apiKey.value.trim();
  if (trimmedKey) {
    update.api_key = trimmedKey;
  }
  emit('save', update);
}
</script>

<template>
  <NForm class="llm-config-form" label-placement="top" @submit.prevent="submit">
    <NFormItem label="Provider">
      <NSelect v-model:value="draft.provider" :options="providerOptions" />
    </NFormItem>

    <NFormItem label="Modell">
      <NSelect
        v-model:value="draft.model"
        :options="modelOptions"
        filterable
        tag
        placeholder="Modell wählen oder eigene ID eingeben"
      />
    </NFormItem>

    <NFormItem label="API-Key">
      <NInput
        v-model:value="apiKey"
        type="password"
        show-password-on="click"
        autocomplete="off"
        :placeholder="apiKeyPlaceholder"
      />
    </NFormItem>

    <NFormItem label="Base URL (optional)">
      <NInput v-model:value="draft.base_url" placeholder="https://api.anthropic.com" clearable />
    </NFormItem>

    <NFormItem label="Timeout (Sekunden)">
      <NInputNumber v-model:value="draft.timeout_seconds" :min="1" :max="600" />
    </NFormItem>

    <NFormItem label="Max. Wiederholungen bei Parse-Fehlern">
      <NInputNumber v-model:value="draft.max_retries" :min="0" :max="10" />
    </NFormItem>

    <NFormItem label="Max. Tool-Aufrufe pro Turn">
      <NInputNumber v-model:value="draft.max_tool_calls_per_turn" :min="1" :max="100" />
    </NFormItem>

    <NFormItem label="Max. LLM-Roundtrips pro Turn">
      <NInputNumber v-model:value="draft.max_llm_roundtrips" :min="1" :max="50" />
    </NFormItem>

    <NFormItem label="Antwortsprache (Chat und Bewertungen)">
      <NInput v-model:value="draft.reasoning_language" placeholder="de" />
    </NFormItem>

    <NButton type="primary" attr-type="submit" :disabled="!canSave" :loading="saving">
      Speichern
    </NButton>
  </NForm>
</template>

<style scoped>
.llm-config-form {
  max-width: 480px;
}
</style>
