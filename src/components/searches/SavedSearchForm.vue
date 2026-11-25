<script setup lang="ts">
/**
 * Create/edit form of a saved portal search (spec 01-17). Presentational: it validates nothing
 * the backend does not also check, and hands the input to its parent, which calls the API.
 */
import { NButton, NFormItem, NInput, NInputNumber, NModal, NSpace, NSwitch, NText } from 'naive-ui';
import { reactive, watch } from 'vue';

import type { SavedSearchInput } from '@/api/savedSearches';
import type { SavedSearch } from '@/types/savedSearch';

const props = defineProps<{
  /** The search to edit; `null` opens an empty form for a new one. */
  search: SavedSearch | null;
  saving: boolean;
}>();

const show = defineModel<boolean>('show', { required: true });

const emit = defineEmits<{
  save: [input: SavedSearchInput];
}>();

function blankDraft() {
  return {
    name: '',
    url: '',
    link_contains: '',
    page_param: '',
    max_pages: 1,
    interval_hours: 24,
    enabled: true,
    import_new: true,
    max_import: 3,
  };
}

const draft = reactive(blankDraft());

/** Refill the form each time it opens, so a cancelled edit does not leak into the next one. */
watch(show, (open) => {
  if (!open) return;
  const { search } = props;
  Object.assign(
    draft,
    search
      ? {
          name: search.name,
          url: search.url,
          link_contains: search.link_contains,
          page_param: search.page_param,
          max_pages: search.max_pages,
          interval_hours: search.interval_hours,
          enabled: search.enabled,
          import_new: search.import_new,
          max_import: search.max_import,
        }
      : blankDraft(),
  );
});

function submit() {
  if (!draft.name.trim() || !draft.url.trim() || props.saving) return;
  emit('save', {
    name: draft.name.trim(),
    url: draft.url.trim(),
    link_contains: draft.link_contains.trim(),
    page_param: draft.page_param.trim(),
    max_pages: draft.max_pages,
    interval_hours: draft.interval_hours,
    enabled: draft.enabled,
    import_new: draft.import_new,
    max_import: draft.max_import,
  });
}
</script>

<template>
  <NModal
    v-model:show="show"
    preset="card"
    :title="search ? 'Suche bearbeiten' : 'Suche speichern'"
    class="saved-search-form"
    :mask-closable="!saving"
  >
    <form @submit.prevent="submit">
      <NFormItem label="Name" required>
        <NInput v-model:value="draft.name" placeholder="z. B. Algarve bis 300.000 €" />
      </NFormItem>
      <NFormItem label="URL der Suchergebnis-Seite" required>
        <NInput v-model:value="draft.url" placeholder="https://…" />
      </NFormItem>
      <NFormItem label="Link enthält (optional)">
        <NInput v-model:value="draft.link_contains" placeholder="z. B. /bostad/" />
      </NFormItem>
      <NSpace>
        <NFormItem label="Seiten (1–5)">
          <NInputNumber v-model:value="draft.max_pages" :min="1" :max="5" />
        </NFormItem>
        <NFormItem label="Seiten-Parameter (ab 2 Seiten nötig)">
          <NInput v-model:value="draft.page_param" placeholder="z. B. page" />
        </NFormItem>
      </NSpace>
      <NFormItem label="Suche alle … Stunden">
        <NInputNumber v-model:value="draft.interval_hours" :min="1" :max="720" />
      </NFormItem>
      <NFormItem label="Aktiv">
        <NSwitch v-model:value="draft.enabled" />
      </NFormItem>
      <NFormItem label="Neue Treffer automatisch vom Agent anlegen und bewerten lassen">
        <NSpace vertical size="small">
          <NSwitch v-model:value="draft.import_new" />
          <NText depth="3">
            Der Agent lädt sie im aktiven Chat des Profils. Das verbraucht LLM-Tokens.
          </NText>
        </NSpace>
      </NFormItem>
      <NFormItem v-if="draft.import_new" label="Höchstens … Inserate pro Lauf (1–10)">
        <NInputNumber v-model:value="draft.max_import" :min="1" :max="10" />
      </NFormItem>
      <NSpace justify="end">
        <NButton :disabled="saving" @click="show = false">Abbrechen</NButton>
        <NButton
          attr-type="submit"
          type="primary"
          :loading="saving"
          :disabled="!draft.name.trim() || !draft.url.trim()"
        >
          Speichern
        </NButton>
      </NSpace>
    </form>
  </NModal>
</template>

<style scoped>
.saved-search-form {
  max-width: 36rem;
}
</style>
