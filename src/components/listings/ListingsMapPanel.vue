<script setup lang="ts">
import { NAlert, NSpin } from 'naive-ui';
import { onMounted, shallowRef } from 'vue';

import ListingMap from '@/components/listings/ListingMap.vue';
import ListingSplitPane from '@/components/listings/ListingSplitPane.vue';
import { useListings } from '@/composables/useListings';

const props = defineProps<{
  profileId: string;
}>();

// Fetches its own listing set independently, same as ListingsPanel.vue's
// "Liste" tab — each ProfileShellView tab owns its own data (spec 01-06),
// no cross-tab coupling.
const { listings, loading, error, fetchListings } = useListings(props.profileId);

const selectedListingId = shallowRef<string | null>(null);

onMounted(fetchListings);
</script>

<template>
  <div class="listings-map-panel">
    <NAlert v-if="error" type="error" :title="error" class="listings-map-panel-error" />

    <NSpin :show="loading">
      <ListingSplitPane :selected-listing-id="selectedListingId">
        <ListingMap
          :listings="listings"
          :selected-listing-id="selectedListingId"
          @select="(id) => (selectedListingId = id)"
        />
      </ListingSplitPane>
    </NSpin>
  </div>
</template>

<style scoped>
.listings-map-panel-error {
  margin-bottom: 1rem;
}
</style>
