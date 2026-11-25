<script setup lang="ts">
import {
  NButton,
  NCheckbox,
  NDataTable,
  NIcon,
  NInputNumber,
  NPopover,
  NSelect,
  NSpace,
  NTag,
  NTooltip,
} from 'naive-ui';
import { computed, h, onMounted, shallowRef } from 'vue';

import { listCriteria } from '@/api/criteria';
import { useListingGrades } from '@/composables/useListingGrades';
import { useListingHazards } from '@/composables/useListingHazards';
import { useListingThumbnails } from '@/composables/useListingThumbnails';
import { gradeColor } from '@/utils/gradeColor';
import {
  hazardLabel,
  hazardMaxRatingOptions,
  hazardRatingColor,
  hazardRatingLabel,
  hazardTypeOptions,
  hazardTypes,
  ratingMap,
} from '@/utils/hazard';
import { distinctRegions, matchesListingFilters } from '@/utils/listingFilters';
import { statusLabel, statusOptions } from '@/utils/listingStatus';

import type { Criterion } from '@/types/criterion';
import type { HazardType } from '@/types/hazard';
import type { Listing, ListingStatus } from '@/types/listing';
import type { ListingFilters } from '@/utils/listingFilters';
import type { DataTableColumns } from 'naive-ui';

const props = defineProps<{
  profileId: string;
  listings: readonly Listing[];
  selectedListingId?: string | null;
}>();

const emit = defineEmits<{
  select: [listingId: string];
}>();

const statusFilter = shallowRef<ListingStatus | null>(null);
const regionFilter = shallowRef<string | null>(null);
const scoreMin = shallowRef<number | null>(null);
const scoreMax = shallowRef<number | null>(null);
const priceMin = shallowRef<number | null>(null);
const priceMax = shallowRef<number | null>(null);
const sizeMin = shallowRef<number | null>(null);
const sizeMax = shallowRef<number | null>(null);
const hazardTypeFilter = shallowRef<HazardType | null>(null);
const hazardMaxFilter = shallowRef<number | null>(null);

// Hazard ratings belong to a region, not to a listing (ADR 0025); each listing's region is
// looked up from its free-text `region` (spec 01-13, issue #45).
const { hazards } = useListingHazards(() => props.listings);

function hazardRating(listing: Listing, type: HazardType): number | undefined {
  return ratingMap(hazards.value.get(listing.id))[type];
}

// Region is free text on the listing (no structured state/region column), so
// the dropdown offers the distinct values present in this profile's listings.
const regionOptions = computed(() =>
  distinctRegions(props.listings).map((region) => ({ label: region, value: region })),
);

const filteredListings = computed(() => {
  const filters: ListingFilters = {
    status: statusFilter.value,
    region: regionFilter.value,
    score: { min: scoreMin.value, max: scoreMax.value },
    price: { min: priceMin.value, max: priceMax.value },
    size: { min: sizeMin.value, max: sizeMax.value },
    hazard: { type: hazardTypeFilter.value, max: hazardMaxFilter.value },
  };
  return props.listings.filter((listing) =>
    matchesListingFilters(listing, filters, ratingMap(hazards.value.get(listing.id))),
  );
});

function selectListing(listingId: string) {
  emit('select', listingId);
}

const { thumbnails } = useListingThumbnails(() => props.listings);

// Optional per-criterion score columns (review #6, spec 01-10). Off by
// default (no bulk grades endpoint — turning one on means an N+1 fetch, see
// useListingGrades) and persisted per profile, since criteria differ
// between profiles and a saved column set from one wouldn't make sense on
// another.
const criteria = shallowRef<Criterion[]>([]);
const visibleCriterionIds = shallowRef<Set<string>>(new Set());
const columnsStorageKey = `listing-list-grade-columns:${props.profileId}`;

onMounted(async () => {
  criteria.value = await listCriteria(props.profileId);
  const stored = localStorage.getItem(columnsStorageKey);
  if (stored) {
    try {
      visibleCriterionIds.value = new Set(JSON.parse(stored) as string[]);
    } catch {
      visibleCriterionIds.value = new Set();
    }
  }
});

function toggleCriterionColumn(criterionId: string, visible: boolean) {
  const next = new Set(visibleCriterionIds.value);
  if (visible) {
    next.add(criterionId);
  } else {
    next.delete(criterionId);
  }
  visibleCriterionIds.value = next;
  localStorage.setItem(columnsStorageKey, JSON.stringify([...next]));
}

const { grades } = useListingGrades(
  () => props.listings,
  () => visibleCriterionIds.value.size > 0,
);

const columns = computed<DataTableColumns<Listing>>(() => [
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
    render: (row) => h(NTag, { size: 'small' }, { default: () => statusLabel[row.status] }),
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
  // One column per hazard type, apart from the Score and never combined: the four ratings use
  // different scales (spec 01-13). Higher is worse. A listing without a rating sorts as 0.
  ...hazardTypes.map((type) => ({
    title: hazardLabel[type],
    key: `hazard:${type}`,
    sorter: (a: Listing, b: Listing) => (hazardRating(a, type) ?? 0) - (hazardRating(b, type) ?? 0),
    render: (row: Listing) => {
      const rating = hazardRating(row, type);
      return rating === undefined
        ? '—'
        : h(
            NTag,
            {
              color: { color: hazardRatingColor[rating], textColor: 'white' },
              round: true,
              title: `${hazardLabel[type]}: ${rating} – ${hazardRatingLabel[rating]}`,
            },
            { default: () => rating },
          );
    },
  })),
  ...criteria.value
    .filter((criterion) => visibleCriterionIds.value.has(criterion.id))
    .map((criterion) => ({
      // Criterion names are often too long for a reasonably narrow column
      // ("Infrastruktur & Erschließung") — rotate the header text instead
      // of letting it force the column wide or wrap awkwardly.
      title: () => h('span', { class: 'listing-list-grade-title' }, criterion.name),
      key: `grade:${criterion.id}`,
      width: 48,
      render: (row: Listing) => {
        const grade = grades.value
          .get(row.id)
          ?.find((entry) => entry.criterion_id === criterion.id)?.grade;
        return grade
          ? h(
              NTag,
              { color: { color: gradeColor[grade], textColor: 'white' }, round: true },
              {
                default: () => grade,
              },
            )
          : '—';
      },
    })),
  {
    title: 'Angelegt',
    key: 'created_at',
    sorter: (a, b) => Date.parse(a.created_at) - Date.parse(b.created_at),
    render: (row) => new Date(row.created_at).toLocaleDateString('de-DE'),
  },
]);
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
      <NSelect
        v-model:value="regionFilter"
        clearable
        placeholder="Region"
        style="width: 180px"
        :options="regionOptions"
        data-testid="filter-region"
      />
      <NInputNumber
        v-model:value="scoreMin"
        :show-button="false"
        placeholder="Score min."
        style="width: 110px"
        data-testid="filter-score-min"
      />
      <NInputNumber
        v-model:value="scoreMax"
        :show-button="false"
        placeholder="Score max."
        style="width: 110px"
        data-testid="filter-score-max"
      />
      <NInputNumber
        v-model:value="priceMin"
        :show-button="false"
        placeholder="Preis min."
        style="width: 120px"
        data-testid="filter-price-min"
      />
      <NInputNumber
        v-model:value="priceMax"
        :show-button="false"
        placeholder="Preis max."
        style="width: 120px"
        data-testid="filter-price-max"
      />
      <NInputNumber
        v-model:value="sizeMin"
        :show-button="false"
        placeholder="Größe min. (m²)"
        style="width: 140px"
        data-testid="filter-size-min"
      />
      <NInputNumber
        v-model:value="sizeMax"
        :show-button="false"
        placeholder="Größe max. (m²)"
        style="width: 140px"
        data-testid="filter-size-max"
      />

      <NSelect
        v-model:value="hazardTypeFilter"
        clearable
        placeholder="Naturgefahr"
        style="width: 150px"
        :options="hazardTypeOptions"
        data-testid="filter-hazard-type"
      />
      <NSelect
        v-model:value="hazardMaxFilter"
        clearable
        placeholder="Einstufung höchstens"
        style="width: 200px"
        :options="hazardMaxRatingOptions"
        data-testid="filter-hazard-max"
      />

      <NPopover trigger="click" placement="bottom-start">
        <template #trigger>
          <NButton>Spalten</NButton>
        </template>
        <NSpace vertical>
          <NCheckbox
            v-for="criterion in criteria"
            :key="criterion.id"
            :checked="visibleCriterionIds.has(criterion.id)"
            :label="criterion.name"
            @update:checked="(checked) => toggleCriterionColumn(criterion.id, checked)"
          />
        </NSpace>
      </NPopover>
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

:deep(.listing-list-grade-title) {
  display: inline-block;
  white-space: nowrap;
  writing-mode: vertical-rl;
  transform: rotate(180deg);
}

:deep(.listing-list-thumb-placeholder) {
  width: 100px;
  height: 100px;
  border-radius: 0.375rem;
  background: var(--n-color-target, rgba(128, 128, 128, 0.15));
}
</style>
