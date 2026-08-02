<script setup lang="ts">
import { NAlert, NButton, NCard, NEmpty, NList, NListItem, NSpin, NTag, NText } from 'naive-ui';
import { computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';

import { useRanking } from '@/composables/useRanking';

import type { Listing } from '@/types/listing';

const props = defineProps<{
  profileId: string;
  listings: readonly Listing[];
}>();

const emit = defineEmits<{
  'listings-updated': [listings: Listing[]];
}>();

const router = useRouter();
const { summary, refreshing, error, fetchSummary, refresh } = useRanking(
  props.profileId,
  (updated) => emit('listings-updated', updated),
);

onMounted(fetchSummary);

// Rank = Score desc, excluding rejected/archived (spec 01-05) — mirrors
// backend ListRankable exactly, computed client-side so it stays in sync
// with whatever the parent's listings array currently holds.
const ranked = computed(() =>
  props.listings
    .filter(
      (listing) =>
        listing.status !== 'rejected' &&
        listing.status !== 'archived' &&
        listing.overall_score !== undefined,
    )
    .sort((a, b) => (b.overall_score ?? 0) - (a.overall_score ?? 0)),
);

function openListing(listingId: string) {
  router.push({ name: 'listing-detail', params: { listingId } });
}
</script>

<template>
  <div class="ranking-view">
    <NCard v-if="summary" title="Gesamt-Einschätzung" class="ranking-summary">
      <NText>{{ summary }}</NText>
    </NCard>

    <NButton :loading="refreshing" @click="refresh"> Rangliste aktualisieren </NButton>
    <NAlert v-if="error" type="error" :title="error" class="ranking-error" />

    <NSpin :show="refreshing">
      <NEmpty
        v-if="ranked.length === 0"
        description="Noch keine bewerteten Inserate für eine Rangliste."
        class="ranking-empty"
      />
      <NList v-else bordered class="ranking-list">
        <NListItem v-for="(listing, index) in ranked" :key="listing.id">
          <div class="ranking-row" @click="openListing(listing.id)">
            <NTag round type="primary" class="ranking-rank">{{ index + 1 }}</NTag>
            <div class="ranking-row-main">
              <NText strong>{{ listing.title }}</NText>
              <NText v-if="listing.kurzfazit" depth="3" class="ranking-kurzfazit">
                {{ listing.kurzfazit }}
              </NText>
            </div>
            <NText strong class="ranking-score">{{ listing.overall_score?.toFixed(0) }}</NText>
          </div>
        </NListItem>
      </NList>
    </NSpin>
  </div>
</template>

<style scoped>
.ranking-summary {
  margin-bottom: 1rem;
}

.ranking-error {
  margin-top: 1rem;
}

.ranking-empty {
  margin-top: 1rem;
}

.ranking-list {
  margin-top: 1rem;
}

.ranking-row {
  display: flex;
  align-items: center;
  gap: 1rem;
  width: 100%;
  cursor: pointer;
}

.ranking-rank {
  flex-shrink: 0;
}

.ranking-row-main {
  display: flex;
  flex-direction: column;
  flex-grow: 1;
}

.ranking-kurzfazit {
  font-size: 0.85rem;
}

.ranking-score {
  flex-shrink: 0;
  font-size: 1.1rem;
}
</style>
