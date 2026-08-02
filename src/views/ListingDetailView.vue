<script setup lang="ts">
import { NButton, NEmpty } from 'naive-ui';
import { useRouter } from 'vue-router';

defineProps<{
  listingId: string;
}>();

const router = useRouter();
</script>

<template>
  <main class="listing-detail-view">
    <NButton text @click="router.back()"> &larr; Zurück </NButton>
    <NEmpty description="Detail-Ansicht kommt in Layer 5.3" />
  </main>
</template>

<style scoped>
.listing-detail-view {
  max-width: 960px;
  margin: 0 auto;
  padding: 2rem 1rem;
}
</style>
