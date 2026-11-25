<script setup lang="ts">
/**
 * Hazard data sources (ADR 0027, spec 01-15): the Agent's source suggestions waiting for the
 * user's decision, and the catalogue of known sources per country. Approving a suggestion only
 * clears it for ingestion. The ratings are static data shipped with a release (ADR 0025), so
 * nothing changes in the app until a maintainer adds the dataset.
 */
import { NAlert, NButton, NH2, NSpace, NTable, NTag, NText, useMessage } from 'naive-ui';
import { computed, onMounted } from 'vue';

import { useHazardSources } from '@/composables/useHazardSources';
import { hazardLabel } from '@/utils/hazard';

import type { HazardSuggestionStatus } from '@/types/hazard';

const message = useMessage();
const { catalog, suggestions, loading, error, fetchAll, approve, reject } = useHazardSources();

onMounted(fetchAll);

const statusLabel: Record<HazardSuggestionStatus, string> = {
  proposed: 'Vorgeschlagen',
  approved: 'Freigegeben – wartet auf Einbindung im nächsten Release',
  rejected: 'Abgelehnt',
};

const statusType: Record<HazardSuggestionStatus, 'warning' | 'success' | 'default'> = {
  proposed: 'warning',
  approved: 'success',
  rejected: 'default',
};

const openSuggestions = computed(() => suggestions.value.filter((s) => s.status === 'proposed'));
const decidedSuggestions = computed(() => suggestions.value.filter((s) => s.status !== 'proposed'));

async function decide(action: typeof approve, suggestionId: string) {
  try {
    await action(suggestionId);
  } catch (decideError) {
    message.error(decideError instanceof Error ? decideError.message : String(decideError));
  }
}
</script>

<template>
  <section class="hazard-sources">
    <NH2>Naturgefahren-Quellen</NH2>
    <NText depth="3" tag="p">
      Die Einstufungen der Regionen stammen aus Datensätzen, die mit der App ausgeliefert werden.
      Der Agent kann neue öffentliche Quellen vorschlagen. Eine Freigabe erlaubt nur, die Quelle in
      einem kommenden Release einzubinden – sie ändert noch keine Einstufung.
    </NText>

    <NAlert v-if="error" type="error" class="hazard-sources-alert">{{ error }}</NAlert>

    <h3>Vorschläge des Agents</h3>
    <NText v-if="!loading && suggestions.length === 0" depth="3" tag="p">
      Noch keine Vorschläge.
    </NText>
    <ul v-else class="hazard-sources-list">
      <li
        v-for="suggestion in [...openSuggestions, ...decidedSuggestions]"
        :key="suggestion.id"
        class="hazard-sources-suggestion"
      >
        <NSpace align="center" size="small">
          <NText strong>{{ suggestion.title }}</NText>
          <NTag :type="statusType[suggestion.status]" size="small">
            {{ statusLabel[suggestion.status] }}
          </NTag>
        </NSpace>
        <div>
          {{ suggestion.country }} · {{ hazardLabel[suggestion.hazard_type] }}
          <template v-if="suggestion.scope"> · {{ suggestion.scope }}</template>
          · {{ suggestion.publisher }}
          <template v-if="suggestion.license"> · Lizenz: {{ suggestion.license }}</template>
        </div>
        <a :href="suggestion.url" target="_blank" rel="noopener noreferrer">{{ suggestion.url }}</a>
        <NText depth="3" tag="p">{{ suggestion.rationale }}</NText>
        <NSpace v-if="suggestion.status === 'proposed'" size="small">
          <NButton size="small" type="primary" @click="decide(approve, suggestion.id)">
            Freigeben
          </NButton>
          <NButton size="small" @click="decide(reject, suggestion.id)">Ablehnen</NButton>
        </NSpace>
      </li>
    </ul>

    <h3>Quellenkatalog</h3>
    <NTable :bordered="false" :single-line="false" size="small">
      <thead>
        <tr>
          <th>Land</th>
          <th>Gefahr</th>
          <th>Gebiet</th>
          <th>Quelle</th>
          <th>Stand</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="entry in catalog" :key="entry.id">
          <td>{{ entry.country }}</td>
          <td>{{ hazardLabel[entry.hazard_type] }}</td>
          <td>{{ entry.scope }}</td>
          <td>
            <a :href="entry.url" target="_blank" rel="noopener noreferrer">{{ entry.title }}</a>
            <div>
              <NText depth="3">{{ entry.publisher }}</NText>
            </div>
            <div v-if="entry.status === 'candidate'">
              <NText depth="3">{{ entry.notes }}</NText>
            </div>
          </td>
          <td>
            <NTag :type="entry.status === 'bundled' ? 'success' : 'default'" size="small">
              {{ entry.status === 'bundled' ? 'Eingebunden' : 'Nicht eingebunden' }}
            </NTag>
          </td>
        </tr>
      </tbody>
    </NTable>
  </section>
</template>

<style scoped>
.hazard-sources {
  margin-top: 2rem;
}

.hazard-sources-alert {
  margin-bottom: 1rem;
}

.hazard-sources-list {
  list-style: none;
  padding: 0;
  margin: 0;
}

.hazard-sources-suggestion {
  padding: 0.75rem 0;
  border-bottom: 1px solid var(--n-border-color, #e0e0e6);
}
</style>
