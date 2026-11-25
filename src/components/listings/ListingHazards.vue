<script setup lang="ts">
import { NEmpty, NTable, NTag, NTooltip } from 'naive-ui';
import { computed } from 'vue';

import { useListingHazards } from '@/composables/useListingHazards';
import { hazardLabel, hazardRatingColor, hazardRatingLabel, hazardTypes } from '@/utils/hazard';

import type { Listing } from '@/types/listing';

/**
 * Natural-hazard ratings of the listing's region (spec 01-13, issue #45). Shown apart from the
 * Score: a rating describes the region, higher is worse, and it never feeds the evaluation
 * (ADR 0025). The four ratings come from four methods, so they are listed side by side and
 * never combined.
 */
const props = defineProps<{
  listing: Pick<Listing, 'id' | 'region' | 'country' | 'address'>;
}>();

const { hazards } = useListingHazards(() => [props.listing]);

const region = computed(() => hazards.value.get(props.listing.id));

const rows = computed(() =>
  hazardTypes.map((type) => ({
    type,
    detail: region.value?.ratings.find((rating) => rating.hazard_type === type),
  })),
);

const regionTitle = computed(() =>
  region.value
    ? region.value.name === region.value.parent_name
      ? region.value.name
      : `${region.value.name} (${region.value.parent_name})`
    : '',
);
</script>

<template>
  <NEmpty
    v-if="!region"
    description="Keine Gefahrendaten: Die Region des Inserats ist unbekannt oder nicht erfasst."
  />
  <div v-else class="listing-hazards">
    <p class="listing-hazards-intro">
      Region: <strong>{{ regionTitle }}</strong
      >. Einstufung von 1 (sehr niedrig) bis 5 (sehr hoch), höher ist schlechter. Sie gilt für die
      ganze Region, nicht für dieses Grundstück, und fließt nicht in den Score ein.
    </p>
    <NTable :bordered="false" :single-line="false" size="small">
      <thead>
        <tr>
          <th>Gefahr</th>
          <th>Einstufung</th>
          <th>Quelle</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="row.type">
          <td>{{ hazardLabel[row.type] }}</td>
          <template v-if="row.detail">
            <td>
              <NTag
                :color="{ color: hazardRatingColor[row.detail.rating], textColor: 'white' }"
                round
                :data-testid="`hazard-badge-${row.type}`"
              >
                {{ row.detail.rating }} – {{ hazardRatingLabel[row.detail.rating] }}
              </NTag>
            </td>
            <td>
              <NTooltip trigger="hover" style="max-width: 420px">
                <template #trigger>
                  <a :href="row.detail.source.url" target="_blank" rel="noopener noreferrer">
                    {{ row.detail.source.publisher }}
                  </a>
                </template>
                <div>{{ row.detail.source.title }} ({{ row.detail.source.dataset_version }})</div>
                <div>Umrechnung auf 1–5: {{ row.detail.source.method }}</div>
              </NTooltip>
            </td>
          </template>
          <td v-else colspan="2">Keine Daten für diese Region</td>
        </tr>
      </tbody>
    </NTable>
  </div>
</template>

<style scoped>
.listing-hazards-intro {
  margin-top: 0;
}
</style>
