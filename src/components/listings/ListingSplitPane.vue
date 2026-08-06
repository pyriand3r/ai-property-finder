<script setup lang="ts">
import { NEmpty } from 'naive-ui';

import ListingDetailPanel from '@/components/listings/ListingDetailPanel.vue';

// Shared master-detail split layout: left slot content (a table, a map, ...)
// stays visible while the selected Listing's full ListingDetailPanel renders
// on the right. Extracted out of ListingsPanel.vue's "Liste" tab once Map
// View (spec 01-06) needed the identical pattern — same sticky/scroll
// behavior, same :key-forced-remount fix, same empty state.
//
// Known issue: the right pane's scroll falls through to page scroll near
// the bottom of long content — see docs/buglist.md "Split-pane detail
// scroll falls through to page scroll near the bottom". Not fixed here.
defineProps<{
  selectedListingId: string | null;
}>();
</script>

<template>
  <div class="listing-split-pane">
    <div class="listing-split-pane-left">
      <slot />
    </div>
    <div class="listing-split-pane-right">
      <ListingDetailPanel
        v-if="selectedListingId"
        :key="selectedListingId"
        :listing-id="selectedListingId"
      />
      <NEmpty v-else description="Listing auswählen" class="listing-split-pane-empty" />
    </div>
  </div>
</template>

<style scoped>
.listing-split-pane {
  display: flex;
  gap: 1.5rem;
  align-items: flex-start;
}

.listing-split-pane-left {
  flex: 1 1 55%;
  min-width: 0;
}

.listing-split-pane-right {
  flex: 1 1 45%;
  min-width: 0;
  position: sticky;
  top: 1rem;
  max-height: calc(100vh - 2rem);
  overflow-y: auto;
}

.listing-split-pane-empty {
  margin-top: 3rem;
}
</style>
