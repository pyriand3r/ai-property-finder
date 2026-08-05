<script setup lang="ts">
import { NButton } from 'naive-ui';
import { useRouter } from 'vue-router';

import ListingDetailPanel from '@/components/listings/ListingDetailPanel.vue';

defineProps<{
  listingId: string;
}>();

const router = useRouter();
</script>

<template>
  <main class="listing-detail-view">
    <NButton text @click="router.back()"> &larr; Zurück </NButton>

    <ListingDetailPanel :listing-id="listingId" />
  </main>
</template>

<style scoped>
.listing-detail-view {
  padding: 2rem 2rem;
}
</style>
