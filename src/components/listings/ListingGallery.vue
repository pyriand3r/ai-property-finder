<script setup lang="ts">
import { NAlert, NButton, NEmpty, NSpin, NTag, useDialog } from 'naive-ui';
import { onMounted } from 'vue';

import { useListingImages } from '@/composables/useListingImages';

import type { ListingImage } from '@/types/listing';

const props = defineProps<{
  listingId: string;
  titleImageId?: string;
}>();

const emit = defineEmits<{
  changed: [];
}>();

const dialog = useDialog();

const { images, loading, error, fetchImages, removeImage, setTitleImage } = useListingImages(
  props.listingId,
);

onMounted(fetchImages);

const categoryLabel: Record<string, string> = {
  exterior: 'Außenansicht',
  interior: 'Innenaufnahme',
  floor_plan: 'Grundriss',
  outbuilding: 'Nebengebäude',
  other: 'Sonstiges',
};

async function handleSetTitleImage(image: ListingImage) {
  await setTitleImage(image.id);
  emit('changed');
}

function confirmDelete(image: ListingImage) {
  dialog.warning({
    title: 'Bild löschen?',
    content: 'Die Datei wird unwiderruflich entfernt.',
    positiveText: 'Löschen',
    negativeText: 'Abbrechen',
    onPositiveClick: async () => {
      await removeImage(image.id);
      if (image.id === props.titleImageId) {
        emit('changed');
      }
    },
  });
}
</script>

<template>
  <div class="listing-gallery">
    <NAlert v-if="error" type="error" :title="error" class="listing-gallery-error" />

    <NSpin :show="loading">
      <NEmpty v-if="images.length === 0" description="Noch keine Bilder vorhanden" />
      <div v-else class="listing-gallery-grid">
        <div v-for="image in images" :key="image.id" class="listing-gallery-item">
          <img :src="`/media/${image.file_path}`" class="listing-gallery-thumb" alt="" />
          <NTag v-if="image.category" size="small" class="listing-gallery-badge">
            {{ categoryLabel[image.category] ?? image.category }}
          </NTag>
          <NButton
            size="tiny"
            circle
            :type="image.id === titleImageId ? 'warning' : 'default'"
            class="listing-gallery-star"
            :title="image.id === titleImageId ? 'Titelbild' : 'Als Titelbild festlegen'"
            @click="handleSetTitleImage(image)"
          >
            {{ image.id === titleImageId ? '★' : '☆' }}
          </NButton>
          <NButton
            size="tiny"
            circle
            class="listing-gallery-delete"
            title="Löschen"
            @click="confirmDelete(image)"
          >
            🗑
          </NButton>
        </div>
      </div>
    </NSpin>
  </div>
</template>

<style scoped>
.listing-gallery-error {
  margin-bottom: 1rem;
}

.listing-gallery-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
}

.listing-gallery-item {
  position: relative;
  width: 140px;
  height: 140px;
}

.listing-gallery-thumb {
  width: 100%;
  height: 100%;
  border-radius: 0.5rem;
  object-fit: cover;
  display: block;
}

.listing-gallery-badge {
  position: absolute;
  top: 0.375rem;
  left: 0.375rem;
}

.listing-gallery-star {
  position: absolute;
  top: 0.375rem;
  right: 0.375rem;
}

.listing-gallery-delete {
  position: absolute;
  bottom: 0.375rem;
  right: 0.375rem;
}
</style>
