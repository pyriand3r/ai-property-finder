<script setup lang="ts">
import { NCard, NSkeleton, NTag } from 'naive-ui';
import { onMounted, shallowRef } from 'vue';
import { useRouter } from 'vue-router';

import { listImages } from '@/api/images';
import { getListing } from '@/api/listings';

import type { Listing } from '@/types/listing';

/**
 * Inline Listing card shown under an Agent reply that touched a Listing
 * (spec 00-03 Layer 6.4: chat_message.listing_id -> title/price/thumbnail
 * /score, click opens the Detail View). Fetches its own data — the chat
 * message only carries the id, not a denormalized snapshot, so the card
 * always reflects the Listing's current state even if it's re-rendered
 * long after the turn that created it.
 */
const props = defineProps<{
  listingId: string;
}>();

const router = useRouter();
const listing = shallowRef<Listing | null>(null);
const thumbnailUrl = shallowRef<string | null>(null);
const loading = shallowRef(true);

onMounted(async () => {
  try {
    const [loaded, images] = await Promise.all([
      getListing(props.listingId),
      listImages(props.listingId).catch(() => []),
    ]);
    listing.value = loaded;
    const titleImage = images.find((image) => image.id === loaded.title_image_id) ?? images[0];
    thumbnailUrl.value = titleImage ? `/media/${titleImage.file_path}` : null;
  } finally {
    loading.value = false;
  }
});

function open() {
  void router.push({ name: 'listing-detail', params: { listingId: props.listingId } });
}
</script>

<template>
  <NCard hoverable size="small" class="chat-listing-card" @click="open">
    <NSkeleton v-if="loading" text :repeat="2" />
    <div v-else-if="listing" class="chat-listing-card-body">
      <img v-if="thumbnailUrl" :src="thumbnailUrl" class="chat-listing-card-thumb" alt="" />
      <div class="chat-listing-card-info">
        <strong>{{ listing.title }}</strong>
        <span v-if="listing.price">
          {{ listing.price.toLocaleString('de-DE') }} {{ listing.price_currency }}
        </span>
        <NTag v-if="listing.overall_score !== undefined" size="small">
          Score {{ listing.overall_score.toFixed(0) }}
        </NTag>
      </div>
    </div>
  </NCard>
</template>

<style scoped>
.chat-listing-card {
  cursor: pointer;
  margin-top: 0.5rem;
  max-width: 320px;
}

.chat-listing-card-body {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.chat-listing-card-thumb {
  width: 56px;
  height: 56px;
  flex-shrink: 0;
  border-radius: 0.375rem;
  object-fit: cover;
}

.chat-listing-card-info {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
</style>
