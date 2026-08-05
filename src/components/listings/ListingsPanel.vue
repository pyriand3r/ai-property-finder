<script setup lang="ts">
import { NAlert, NEmpty, NSpin, NTabPane, NTabs } from 'naive-ui';
import { onMounted, shallowRef } from 'vue';

import ListingDetailPanel from '@/components/listings/ListingDetailPanel.vue';
import ListingListTable from '@/components/listings/ListingListTable.vue';
import RankingView from '@/components/listings/RankingView.vue';
import ReevaluationBanner from '@/components/shared/ReevaluationBanner.vue';
import { useListings } from '@/composables/useListings';

const props = defineProps<{
  profileId: string;
}>();

const { listings, loading, error, fetchListings, mergeListings } = useListings(props.profileId);

const selectedListingId = shallowRef<string | null>(null);

onMounted(fetchListings);
</script>

<template>
  <div class="listings-panel">
    <ReevaluationBanner :profile-id="profileId" />
    <NAlert v-if="error" type="error" :title="error" class="listings-panel-error" />

    <NSpin :show="loading">
      <NTabs type="segment" animated>
        <NTabPane name="list" tab="Liste">
          <div class="listings-panel-split">
            <div class="listings-panel-split-list">
              <ListingListTable
                :listings="listings"
                :selected-listing-id="selectedListingId"
                @select="(id) => (selectedListingId = id)"
              />
            </div>
            <div class="listings-panel-split-detail">
              <ListingDetailPanel
                v-if="selectedListingId"
                :key="selectedListingId"
                :listing-id="selectedListingId"
              />
              <NEmpty v-else description="Listing auswählen" class="listings-panel-split-empty" />
            </div>
          </div>
        </NTabPane>
        <NTabPane name="ranking" tab="Rangliste">
          <RankingView
            :profile-id="profileId"
            :listings="listings"
            @listings-updated="mergeListings"
          />
        </NTabPane>
      </NTabs>
    </NSpin>
  </div>
</template>

<style scoped>
.listings-panel-error {
  margin-bottom: 1rem;
}

.listings-panel-split {
  display: flex;
  gap: 1.5rem;
  align-items: flex-start;
}

.listings-panel-split-list {
  flex: 1 1 55%;
  min-width: 0;
}

.listings-panel-split-detail {
  flex: 1 1 45%;
  min-width: 0;
  position: sticky;
  top: 1rem;
  max-height: calc(100vh - 2rem);
  overflow-y: auto;
}

.listings-panel-split-empty {
  margin-top: 3rem;
}
</style>
