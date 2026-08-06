<script setup lang="ts">
import { NAlert, NButton, NH1, NInput, NSelect, NSpin, NTag } from 'naive-ui';
import { computed, onMounted, reactive, watch } from 'vue';

import ListingDocument from '@/components/listings/ListingDocument.vue';
import ListingForm from '@/components/listings/ListingForm.vue';
import ListingGradeGrid from '@/components/listings/ListingGradeGrid.vue';
import { useListingDetail } from '@/composables/useListingDetail';
import { useListingThumbnails } from '@/composables/useListingThumbnails';

import type { ListingInput } from '@/api/listings';
import type { ListingStatus } from '@/types/listing';

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

const statusOptions: { label: string; value: ListingStatus }[] = [
  { label: 'Neu', value: 'new' },
  { label: 'Wartend', value: 'waiting' },
  { label: 'In Verhandlung', value: 'in_negotiation' },
  { label: 'Besichtigt', value: 'visited' },
  { label: 'Abgelehnt', value: 'rejected' },
  { label: 'Archiviert', value: 'archived' },
];

// status/rejection_comment moved out of ListingForm's field grid into the
// header (spec 01-10 — "prominent, same tier as needs_reevaluation") and
// stay immediate-save (unlike the rest of the form's fields, which now
// batch on Bearbeiten/Speichern) — a status change is a quick, standalone
// workflow action, not something users batch with other edits.
const statusDraft = reactive({
  status: '' as ListingStatus | '',
  rejection_comment: undefined as string | undefined,
});

watch(
  listing,
  (current) => {
    if (current) {
      statusDraft.status = current.status;
      statusDraft.rejection_comment = current.rejection_comment;
    }
  },
  { immediate: true },
);

function saveStatus() {
  void saveField({
    status: statusDraft.status || undefined,
    rejection_comment: statusDraft.rejection_comment,
  });
}
</script>

<template>
  <div class="listing-detail-panel">
    <NAlert v-if="error" type="error" :title="error" class="listing-detail-error" />

    <NSpin :show="loading || saving">
      <template v-if="listing">
        <div class="listing-detail-header">
          <NH1>{{ listing.title }}</NH1>
          <NSelect
            v-model:value="statusDraft.status"
            :options="statusOptions"
            style="width: 160px"
            @update:value="saveStatus"
          />
          <NInput
            v-if="statusDraft.status === 'rejected'"
            v-model:value="statusDraft.rejection_comment"
            placeholder="Ablehnungsgrund"
            style="width: 220px"
            @blur="saveStatus"
          />
          <NTag v-if="listing.needs_reevaluation" type="warning">Bewertung veraltet</NTag>
          <NButton type="primary" :loading="evaluating" @click="handleEvaluate">
            Neu bewerten
          </NButton>
        </div>

        <img v-if="thumbnailUrl" :src="thumbnailUrl" class="listing-detail-thumb" alt="" />

        <section class="listing-detail-section">
          <h3>Bewertung</h3>
          <ListingGradeGrid :grades="grades" />
        </section>

        <section class="listing-detail-section">
          <ListingForm :listing="listing" @save="handleSave" />
        </section>

        <section class="listing-detail-section">
          <ListingDocument
            :document-md="listing.document_md"
            @save="(text) => handleSave({ document_md: text })"
          />
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
  flex-wrap: wrap;
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
