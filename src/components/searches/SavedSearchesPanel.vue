<script setup lang="ts">
/**
 * Saved portal searches of a profile (spec 01-17, ADR 0029): the URL, the schedule, whether new
 * links are imported by the Agent, and the outcome of the last run. The scheduler runs on the
 * backend; this tab only edits the saved searches and can start a run right away.
 */
import {
  NAlert,
  NButton,
  NPopconfirm,
  NSpace,
  NSwitch,
  NTable,
  NTag,
  NText,
  useMessage,
} from 'naive-ui';
import { onMounted, shallowRef } from 'vue';

import SavedSearchForm from '@/components/searches/SavedSearchForm.vue';
import { useSavedSearches } from '@/composables/useSavedSearches';
import {
  formatDateTime,
  intervalLabel,
  savedSearchStatusLabel,
  savedSearchStatusType,
} from '@/utils/savedSearch';

import type { SavedSearchInput } from '@/api/savedSearches';
import type { SavedSearch } from '@/types/savedSearch';

const props = defineProps<{
  profileId: string;
}>();

const message = useMessage();
const {
  searches,
  loading,
  error,
  fetchSearches,
  createSearch,
  updateSearch,
  deleteSearch,
  runSearch,
} = useSavedSearches(props.profileId);

onMounted(() => fetchSearches());

const showForm = shallowRef(false);
const editing = shallowRef<SavedSearch | null>(null);
const saving = shallowRef(false);

function openCreate() {
  editing.value = null;
  showForm.value = true;
}

function openEdit(search: SavedSearch) {
  editing.value = search;
  showForm.value = true;
}

/** Runs a mutation and shows a failure as a toast (the one-off-action error pattern). */
async function attempt(action: () => Promise<unknown>) {
  try {
    await action();
    return true;
  } catch (actionError) {
    message.error(actionError instanceof Error ? actionError.message : String(actionError));
    return false;
  }
}

async function handleSave(input: SavedSearchInput) {
  saving.value = true;
  const current = editing.value;
  const saved = await attempt(() =>
    current ? updateSearch(current.id, input) : createSearch(input),
  );
  saving.value = false;
  if (saved) showForm.value = false;
}

async function handleToggle(search: SavedSearch, enabled: boolean) {
  await attempt(() => updateSearch(search.id, { enabled }));
}

async function handleRun(search: SavedSearch) {
  await attempt(() => runSearch(search.id));
}

async function handleDelete(search: SavedSearch) {
  await attempt(() => deleteSearch(search.id));
}
</script>

<template>
  <section class="saved-searches">
    <NSpace justify="space-between" align="center">
      <NText depth="3">
        Gespeicherte Suchergebnis-Seiten werden regelmäßig durchsucht. Neue Treffer meldet die App
        und lässt sie auf Wunsch vom Agent als Inserate anlegen.
      </NText>
      <NButton type="primary" @click="openCreate">Suche speichern</NButton>
    </NSpace>

    <NAlert v-if="error" type="error" class="saved-searches-alert">{{ error }}</NAlert>

    <NText v-if="!loading && searches.length === 0" depth="3" tag="p">
      Noch keine gespeicherten Suchen.
    </NText>

    <NTable v-else :bordered="false" :single-line="false" size="small" class="saved-searches-table">
      <thead>
        <tr>
          <th>Suche</th>
          <th>Zeitplan</th>
          <th>Aktiv</th>
          <th>Letzter Lauf</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="search in searches" :key="search.id" class="saved-searches-row">
          <td>
            <NText strong>{{ search.name }}</NText>
            <div>
              <a :href="search.url" target="_blank" rel="noopener noreferrer">{{ search.url }}</a>
            </div>
            <NText depth="3">
              {{
                search.import_new ? `Import: bis zu ${search.max_import} pro Lauf` : 'Nur melden'
              }}
            </NText>
          </td>
          <td>
            {{ intervalLabel(search.interval_hours) }}
            <div>
              <NText depth="3">Nächster Lauf: {{ formatDateTime(search.next_run_at) }}</NText>
            </div>
          </td>
          <td>
            <NSwitch
              :value="search.enabled"
              :aria-label="`${search.name} aktiv`"
              @update:value="(enabled: boolean) => handleToggle(search, enabled)"
            />
          </td>
          <td>
            <NTag :type="savedSearchStatusType[search.last_status]" size="small">
              {{ savedSearchStatusLabel[search.last_status] }}
            </NTag>
            <NText v-if="search.last_run_at" depth="3">
              {{ ' ' }}{{ formatDateTime(search.last_run_at) }}
            </NText>
            <div v-if="search.last_status === 'failed' && search.last_error">
              <NText type="error">{{ search.last_error }}</NText>
            </div>
            <div v-else-if="search.last_summary">
              <NText depth="3">{{ search.last_summary }}</NText>
            </div>
          </td>
          <td>
            <NSpace size="small">
              <NButton
                size="small"
                :disabled="search.last_status === 'running'"
                @click="handleRun(search)"
              >
                Jetzt suchen
              </NButton>
              <NButton size="small" @click="openEdit(search)">Bearbeiten</NButton>
              <NPopconfirm @positive-click="handleDelete(search)">
                <template #trigger>
                  <NButton size="small" type="error" quaternary>Löschen</NButton>
                </template>
                Gespeicherte Suche löschen? Bereits angelegte Inserate bleiben.
              </NPopconfirm>
            </NSpace>
          </td>
        </tr>
      </tbody>
    </NTable>

    <SavedSearchForm
      v-model:show="showForm"
      :search="editing"
      :saving="saving"
      @save="handleSave"
    />
  </section>
</template>

<style scoped>
.saved-searches-alert,
.saved-searches-table {
  margin-top: 1rem;
}
</style>
