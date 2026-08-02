<script setup lang="ts">
import { NAlert, NButton, NH1, NSpin, NTag } from 'naive-ui';
import { onMounted } from 'vue';
import { useRouter } from 'vue-router';

import ListingDocument from '@/components/listings/ListingDocument.vue';
import ListingForm from '@/components/listings/ListingForm.vue';
import ListingGradeGrid from '@/components/listings/ListingGradeGrid.vue';
import { useListingDetail } from '@/composables/useListingDetail';

import type { ListingInput } from '@/api/listings';

const props = defineProps<{
  listingId: string;
}>();

const router = useRouter();
const { listing, grades, loading, saving, evaluating, error, fetchAll, saveField, evaluate } =
  useListingDetail(props.listingId);

onMounted(fetchAll);

function handleSave(patch: ListingInput) {
  void saveField(patch);
}

function handleEvaluate() {
  void evaluate();
}
</script>

<template>
  <main class="listing-detail-view">
    <NButton text @click="router.back()"> &larr; Zurück </NButton>

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
  </main>
</template>

<style scoped>
.listing-detail-view {
  max-width: 960px;
  margin: 0 auto;
  padding: 2rem 1rem;
}

.listing-detail-error {
  margin-block: 1rem;
}

.listing-detail-header {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.listing-detail-section {
  margin-top: 2rem;
}
</style>
