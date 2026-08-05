<script setup lang="ts">
import { NAlert, NButton, NH1, NSpin, NTag } from 'naive-ui';
import { computed, onMounted } from 'vue';

import ListingDocument from '@/components/listings/ListingDocument.vue';
import ListingForm from '@/components/listings/ListingForm.vue';
import ListingGradeGrid from '@/components/listings/ListingGradeGrid.vue';
import { useListingDetail } from '@/composables/useListingDetail';
import { useListingThumbnails } from '@/composables/useListingThumbnails';

import type { ListingInput } from '@/api/listings';

// useListingDetail captures listingId once, at call time — a prop change on
// an already-mounted instance would not refetch. Callers that swap listingId
// on a live instance (e.g. the master-detail split) must force a remount via
// :key="listingId" rather than relying on prop reactivity here.
const props = defineProps<{
  listingId: string;
}>();

const { listing, grades, loading, saving, evaluating, error, fetchAll, saveField, evaluate } =
  useListingDetail(props.listingId);

onMounted(fetchAll);

const { thumbnails } = useListingThumbnails(() => (listing.value ? [listing.value] : []));
const thumbnailUrl = computed(() =>
  listing.value ? (thumbnails.value.get(listing.value.id) ?? null) : null,
);

function handleSave(patch: ListingInput) {
  void saveField(patch);
}

function handleEvaluate() {
  void evaluate();
}
</script>

<template>
  <div class="listing-detail-panel">
    <NAlert v-if="error" type="error" :title="error" class="listing-detail-error" />

    <NSpin :show="loading || saving">
      <template v-if="listing">
        <div class="listing-detail-header">
          <NH1>{{ listing.title }}</NH1>
          <NTag v-if="listing.needs_reevaluation" type="warning">Bewertung veraltet</NTag>
          <NButton type="primary" :loading="evaluating" @click="handleEvaluate">
            Neu bewerten
          </NButton>
        </div>

        <img v-if="thumbnailUrl" :src="thumbnailUrl" class="listing-detail-thumb" alt="" />

        <section class="listing-detail-section">
          <ListingForm :listing="listing" @save="handleSave" />
        </section>

        <section class="listing-detail-section">
          <ListingDocument
            :document-md="listing.document_md"
            @save="(text) => handleSave({ document_md: text })"
          />
        </section>

        <section class="listing-detail-section">
          <h3>Bewertung</h3>
          <ListingGradeGrid :grades="grades" />
        </section>
      </template>
    </NSpin>
  </div>
</template>

<style scoped>
.listing-detail-error {
  margin-block: 1rem;
}

.listing-detail-header {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.listing-detail-thumb {
  width: 100%;
  max-height: 360px;
  border-radius: 0.5rem;
  object-fit: cover;
  display: block;
  margin-top: 1rem;
}

.listing-detail-section {
  margin-top: 2rem;
}
</style>
