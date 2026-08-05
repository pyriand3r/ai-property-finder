<script setup lang="ts">
import { NDataTable, NIcon, NInputNumber, NSelect, NSpace, NTag, NTooltip } from 'naive-ui';
import { computed, h, shallowRef } from 'vue';

import { useListingThumbnails } from '@/composables/useListingThumbnails';

import type { Listing, ListingStatus } from '@/types/listing';
import type { DataTableColumns } from 'naive-ui';

const props = defineProps<{
  listings: readonly Listing[];
  selectedListingId?: string | null;
}>();

const emit = defineEmits<{
  select: [listingId: string];
}>();

const statusOptions: { label: string; value: ListingStatus }[] = [
  { label: 'Neu', value: 'new' },
  { label: 'Wartend', value: 'waiting' },
  { label: 'In Verhandlung', value: 'in_negotiation' },
  { label: 'Besichtigt', value: 'visited' },
  { label: 'Abgelehnt', value: 'rejected' },
  { label: 'Archiviert', value: 'archived' },
];

const statusFilter = shallowRef<ListingStatus | null>(null);
const scoreMin = shallowRef<number | null>(null);
const priceMax = shallowRef<number | null>(null);

const filteredListings = computed(() =>
  props.listings.filter((listing) => {
    if (statusFilter.value && listing.status !== statusFilter.value) {
      return false;
    }
    if (scoreMin.value !== null && (listing.overall_score ?? -1) < scoreMin.value) {
      return false;
    }
    if (priceMax.value !== null && (listing.price ?? Infinity) > priceMax.value) {
      return false;
    }
    return true;
  }),
);

function selectListing(listingId: string) {
  emit('select', listingId);
}

const { thumbnails } = useListingThumbnails(() => props.listings);

const columns: DataTableColumns<Listing> = [
  {
    title: '',
    key: 'thumbnail',
    width: 108,
    render: (row) => {
      const url = thumbnails.value.get(row.id);
      return url
        ? h('img', { src: url, class: 'listing-list-thumb', alt: '' })
        : h('div', { class: 'listing-list-thumb-placeholder' });
    },
  },
  {
    title: '',
    key: 'needs_reevaluation',
    width: 32,
    render: (row) =>
      row.needs_reevaluation
        ? h(
            NTooltip,
            { trigger: 'hover' },
            {
              trigger: () => h(NIcon, { color: '#f0a020' }, { default: () => '⚠️' }),
              default: () => 'Bewertung veraltet',
            },
          )
        : null,
  },
  {
    title: 'Titel',
    key: 'title',
    sorter: 'default',
    render: (row) =>
      h(
        'a',
        {
          onClick: (event: MouseEvent) => {
            event.stopPropagation();
            selectListing(row.id);
          },
        },
        row.title,
      ),
  },
  {
    title: 'Status',
    key: 'status',
    render: (row) => h(NTag, { size: 'small' }, { default: () => row.status }),
  },
  {
    title: 'Preis',
    key: 'price',
    sorter: (a, b) => (a.price ?? 0) - (b.price ?? 0),
    render: (row) =>
      row.price === undefined
        ? '—'
        : `${row.price.toLocaleString('de-DE')} ${row.price_currency ?? ''}`.trim(),
  },
  {
    title: 'Größe (m²)',
    key: 'property_size_m2',
    sorter: (a, b) => (a.property_size_m2 ?? 0) - (b.property_size_m2 ?? 0),
    render: (row) => row.property_size_m2?.toLocaleString('de-DE') ?? '—',
  },
  {
    title: 'Score',
    key: 'overall_score',
    sorter: (a, b) => (a.overall_score ?? 0) - (b.overall_score ?? 0),
    defaultSortOrder: 'descend',
    render: (row) => row.overall_score?.toFixed(0) ?? '—',
  },
  {
    title: 'Angelegt',
    key: 'created_at',
    sorter: (a, b) => Date.parse(a.created_at) - Date.parse(b.created_at),
    render: (row) => new Date(row.created_at).toLocaleDateString('de-DE'),
  },
];
</script>

<template>
  <div class="listing-list-table">
    <NSpace class="listing-list-filters" align="center">
      <NSelect
        v-model:value="statusFilter"
        clearable
        placeholder="Status"
        style="width: 160px"
        :options="statusOptions"
      />
      <NInputNumber v-model:value="scoreMin" placeholder="Score min." style="width: 140px" />
      <NInputNumber v-model:value="priceMax" placeholder="Preis max." style="width: 140px" />
    </NSpace>

    <NDataTable
      :columns="columns"
      :data="filteredListings"
      :row-key="(row: Listing) => row.id"
      :row-class-name="
        (row: Listing) => (row.id === selectedListingId ? 'listing-list-row-selected' : '')
      "
      :row-props="(row: Listing) => ({ onClick: () => selectListing(row.id) })"
    />
  </div>
</template>

<style scoped>
.listing-list-filters {
  margin-bottom: 1rem;
}

:deep(.listing-list-row-selected td) {
  background-color: var(--n-color-target, rgba(128, 128, 128, 0.15));
}

:deep(.listing-list-thumb) {
  width: 100px;
  height: 100px;
  border-radius: 0.375rem;
  object-fit: cover;
  display: block;
}

:deep(.listing-list-thumb-placeholder) {
  width: 100px;
  height: 100px;
  border-radius: 0.375rem;
  background: var(--n-color-target, rgba(128, 128, 128, 0.15));
}
</style>
