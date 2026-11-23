<script setup lang="ts">
import { NButton, NEmpty, NFormItem, NInput, NList, NListItem, NModal, NSpace } from 'naive-ui';
import { computed, shallowRef } from 'vue';

import { parsePortalRules, upsertPortalRule } from '@/utils/portalRules';

import type { PortalRule } from '@/utils/portalRules';

const props = defineProps<{
  content: string;
  saving: boolean;
}>();

const emit = defineEmits<{
  save: [contentMd: string];
}>();

const showEditor = shallowRef(false);
const originalDomain = shallowRef<string | null>(null);
const domain = shallowRef('');
const ruleText = shallowRef('');

const rules = computed(() => parsePortalRules(props.content));
const normalizedDomain = computed(() => domain.value.trim().toLowerCase());

const domainError = computed(() => {
  if (!normalizedDomain.value) {
    return 'Domain ist erforderlich.';
  }
  if (/[\r\n#]/.test(domain.value)) {
    return 'Domain darf keine Zeilenumbrüche oder #-Zeichen enthalten.';
  }
  const original = originalDomain.value?.trim().toLowerCase();
  if (
    rules.value.some(
      (rule) =>
        rule.domain.trim().toLowerCase() === normalizedDomain.value &&
        rule.domain.trim().toLowerCase() !== original,
    )
  ) {
    return 'Für diese Domain existiert bereits eine Regel.';
  }
  return null;
});

const ruleError = computed(() => {
  if (!ruleText.value.trim()) {
    return 'Regeltext ist erforderlich.';
  }
  if (/^ {0,3}#{1,3}(?:[\t ]+|$)/m.test(ruleText.value)) {
    return 'Überschriften der Ebenen # bis ### sind innerhalb einer Regel nicht erlaubt.';
  }
  return null;
});

const canSubmit = computed(() => !props.saving && !domainError.value && !ruleError.value);

function openAdd() {
  originalDomain.value = null;
  domain.value = '';
  ruleText.value = '';
  showEditor.value = true;
}

function openEdit(rule: PortalRule) {
  originalDomain.value = rule.domain;
  domain.value = rule.domain;
  ruleText.value = rule.text;
  showEditor.value = true;
}

function submit() {
  if (!canSubmit.value) {
    return;
  }
  emit(
    'save',
    upsertPortalRule(props.content, originalDomain.value, normalizedDomain.value, ruleText.value),
  );
  showEditor.value = false;
}
</script>

<template>
  <section class="portal-rules-editor">
    <div class="portal-rules-editor-heading">
      <div>
        <h3>Portal-Regeln</h3>
        <p class="portal-rules-editor-hint">
          Pro Domain eine Anweisung. Alle Regeln bleiben Markdown-Text in dieser Context-Datei.
        </p>
      </div>
      <NButton size="small" type="primary" :disabled="saving" @click="openAdd">
        Regel hinzufügen
      </NButton>
    </div>

    <NEmpty v-if="rules.length === 0" description="Noch keine Portal-Regeln vorhanden" />
    <NList v-else bordered>
      <NListItem v-for="(rule, index) in rules" :key="`${rule.domain}-${index}`">
        <div class="portal-rules-editor-item">
          <div class="portal-rules-editor-rule">
            <strong class="portal-rules-editor-domain">{{ rule.domain }}</strong>
            <div class="portal-rules-editor-text">{{ rule.text }}</div>
          </div>
          <NButton size="small" :disabled="saving" @click="openEdit(rule)">Bearbeiten</NButton>
        </div>
      </NListItem>
    </NList>

    <NModal
      v-model:show="showEditor"
      preset="card"
      :title="originalDomain === null ? 'Portal-Regel hinzufügen' : 'Portal-Regel bearbeiten'"
      class="portal-rules-editor-modal"
      :closable="!saving"
      :mask-closable="!saving"
    >
      <form @submit.prevent="submit">
        <NFormItem
          label="Domain"
          required
          :validation-status="domainError ? 'error' : undefined"
          :feedback="domainError ?? undefined"
        >
          <NInput v-model:value="domain" placeholder="z. B. booli.se" :disabled="saving" />
        </NFormItem>
        <NFormItem
          label="Regel"
          required
          :validation-status="ruleError ? 'error' : undefined"
          :feedback="ruleError ?? undefined"
        >
          <NInput
            v-model:value="ruleText"
            type="textarea"
            :autosize="{ minRows: 4, maxRows: 16 }"
            placeholder="z. B. Folge immer dem Link zur Makler-Seite."
            :disabled="saving"
          />
        </NFormItem>
        <NSpace justify="end">
          <NButton :disabled="saving" @click="showEditor = false">Abbrechen</NButton>
          <NButton attr-type="submit" type="primary" :disabled="!canSubmit" :loading="saving">
            Speichern
          </NButton>
        </NSpace>
      </form>
    </NModal>
  </section>
</template>

<style scoped>
.portal-rules-editor {
  margin-bottom: 2rem;
}

.portal-rules-editor-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 0.75rem;
}

.portal-rules-editor-heading h3 {
  margin: 0 0 0.25rem;
}

.portal-rules-editor-hint {
  margin: 0;
  font-size: 0.85rem;
  opacity: 0.7;
}

.portal-rules-editor-item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  width: 100%;
}

.portal-rules-editor-rule {
  min-width: 0;
}

.portal-rules-editor-domain {
  display: block;
  margin-bottom: 0.35rem;
}

.portal-rules-editor-text {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.portal-rules-editor-modal {
  width: min(36rem, 92vw);
}
</style>
