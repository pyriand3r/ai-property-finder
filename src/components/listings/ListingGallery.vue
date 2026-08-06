<script setup lang="ts">
import { NAlert, NButton, NEmpty, NImage, NImageGroup, NSpin, NTag, useDialog } from 'naive-ui';
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
      <NImageGroup v-else>
        <div class="listing-gallery-grid">
          <div v-for="image in images" :key="image.id" class="listing-gallery-item">
            <NImage
              :src="`/media/${image.file_path}`"
              width="140"
              height="140"
              object-fit="cover"
              class="listing-gallery-thumb"
            />
            <NTag v-if="image.category" size="small" class="listing-gallery-badge">
              {{ categoryLabel[image.category] ?? image.category }}
            </NTag>
            <NButton
              size="tiny"
              circle
              :type="image.id === titleImageId ? 'warning' : 'default'"
              class="listing-gallery-star"
              :class="{ 'listing-gallery-star-unselected': image.id !== titleImageId }"
              :title="image.id === titleImageId ? 'Titelbild' : 'Als Titelbild festlegen'"
              @click.stop="handleSetTitleImage(image)"
            >
              {{ image.id === titleImageId ? '★' : '☆' }}
            </NButton>
            <NButton
              size="tiny"
              circle
              class="listing-gallery-delete"
              title="Löschen"
              @click.stop="confirmDelete(image)"
            >
              🗑
            </NButton>
          </div>
        </div>
      </NImageGroup>
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
  display: block;
  border-radius: 0.5rem;
  overflow: hidden;
}

.listing-gallery-thumb :deep(img) {
  border-radius: 0.5rem;
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

/* Default NButton "default" type is too low-contrast against arbitrary
   photo content — give the unselected star and delete button an always-
   visible dark backdrop, filling with a solid accent color on hover (same
   treatment as the selected/warning-type star already gets from Naive UI). */
.listing-gallery-star-unselected,
.listing-gallery-delete {
  background: rgba(0, 0, 0, 0.45) !important;
  color: #fff !important;
  border: none !important;
}

.listing-gallery-star-unselected:hover {
  background: #f0a020 !important;
}

.listing-gallery-delete:hover {
  background: #d03050 !important;
}
</style>

<!-- Unscoped on purpose: NImageGroup's preview overlay is teleported to
     <body>, outside this component's scoped-style reach. Naive UI hardcodes
     the backdrop at rgba(0,0,0,.3) in its own component styles (not exposed
     as a theme token — checked imageProps/themeOverrides, only toolbar
     colors are configurable), so overriding it needs a plain global rule. -->
<style>
.n-image-preview-container {
  background: rgba(0, 0, 0, 0.85) !important;
}
</style>
