<script setup lang="ts">
import { NAlert, NSpin, NTabPane, NTabs } from 'naive-ui';
import { onMounted } from 'vue';

import ListingListTable from '@/components/listings/ListingListTable.vue';
import RankingView from '@/components/listings/RankingView.vue';
import ReevaluationBanner from '@/components/shared/ReevaluationBanner.vue';
import { useListings } from '@/composables/useListings';

const props = defineProps<{
  profileId: string;
}>();

const { listings, loading, error, fetchListings, mergeListings } = useListings(props.profileId);

onMounted(fetchListings);
</script>

<template>
  <div class="listings-panel">
    <ReevaluationBanner :profile-id="profileId" />
    <NAlert v-if="error" type="error" :title="error" class="listings-panel-error" />

    <NSpin :show="loading">
      <NTabs type="segment" animated>
        <NTabPane name="list" tab="Liste">
          <ListingListTable :listings="listings" />
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
</style>
