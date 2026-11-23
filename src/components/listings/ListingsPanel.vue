<script setup lang="ts">
import { NAlert, NButton, NSpin, NTabPane, NTabs } from 'naive-ui';
import { onMounted, shallowRef } from 'vue';

import ListingCreateModal from '@/components/listings/ListingCreateModal.vue';
import ListingListTable from '@/components/listings/ListingListTable.vue';
import ListingSplitPane from '@/components/listings/ListingSplitPane.vue';
import RankingView from '@/components/listings/RankingView.vue';
import ReevaluationBanner from '@/components/shared/ReevaluationBanner.vue';
import { useListings } from '@/composables/useListings';

const props = defineProps<{
  profileId: string;
}>();

const { listings, loading, error, fetchListings, mergeListings } = useListings(props.profileId);

const selectedListingId = shallowRef<string | null>(null);
const showCreate = shallowRef(false);

async function handleCreated(listingId: string) {
  await fetchListings();
  selectedListingId.value = listingId;
}

onMounted(fetchListings);
</script>

<template>
  <div class="listings-panel">
    <ReevaluationBanner :profile-id="profileId" />
    <NAlert v-if="error" type="error" :title="error" class="listings-panel-error" />

    <div class="listings-panel-toolbar">
      <NButton size="small" @click="showCreate = true">Inserat anlegen</NButton>
    </div>
    <ListingCreateModal
      v-model:show="showCreate"
      :profile-id="profileId"
      @created="handleCreated"
    />

    <NSpin :show="loading">
      <NTabs type="segment" animated>
        <NTabPane name="list" tab="Liste">
          <ListingSplitPane :selected-listing-id="selectedListingId">
            <ListingListTable
              :profile-id="profileId"
              :listings="listings"
              :selected-listing-id="selectedListingId"
              @select="(id) => (selectedListingId = id)"
            />
          </ListingSplitPane>
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
.listings-panel-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 0.5rem;
}

.listings-panel-error {
  margin-bottom: 1rem;
}
</style>
