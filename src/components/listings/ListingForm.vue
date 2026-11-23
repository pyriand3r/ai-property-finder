<script setup lang="ts">
import {
  NButton,
  NDatePicker,
  NFormItem,
  NGrid,
  NGridItem,
  NInput,
  NInputNumber,
  NSpace,
} from 'naive-ui';
import { reactive, ref, watch } from 'vue';

import type { ListingInput } from '@/api/listings';
import type { Listing } from '@/types/listing';

const props = defineProps<{
  listing: Listing;
}>();

const emit = defineEmits<{
  save: [patch: ListingInput];
}>();

// status/rejection_comment are edited in the page header now (spec 01-10 —
// "status moves out of the field grid, prominent next to the title"), not
// here, so they're excluded from both the grid and the diff below.
const editableFields = [
  'title',
  'source_url',
  'price',
  'price_currency',
  'property_size_m2',
  'plot_size_m2',
  'address',
  'region',
  'country',
  'latitude',
  'longitude',
  'description',
  'seller_name',
  'seller_phone',
  'seller_email',
  'last_contact_at',
  'seller_notes',
] as const;

const editing = ref(false);
const draft = reactive({ ...props.listing });

watch(
  () => props.listing,
  (updated) => {
    if (!editing.value) {
      Object.assign(draft, updated);
    }
  },
);

function startEditing() {
  Object.assign(draft, props.listing);
  editing.value = true;
}

function cancel() {
  Object.assign(draft, props.listing);
  editing.value = false;
}

// Batches every changed field into one PATCH on Speichern (spec 01-10 —
// replaces the old save-on-blur-per-field behavior). user_confirmed_fields
// / needs_reevaluation still fire per changed field, just computed once
// here across the whole batch instead of once per blur (backend-side,
// unchanged).
function save() {
  const patch: ListingInput = {};
  for (const field of editableFields) {
    const value = draft[field as keyof typeof draft] as ListingInput[typeof field];
    if (value !== props.listing[field as keyof Listing]) {
      (patch as Record<string, unknown>)[field] = value;
    }
  }
  if (Object.keys(patch).length > 0) {
    emit('save', patch);
  }
  editing.value = false;
}
</script>

<template>
  <div class="listing-form">
    <div class="listing-form-toolbar">
      <NButton v-if="!editing" size="small" @click="startEditing">Bearbeiten</NButton>
      <NSpace v-else size="small">
        <NButton size="small" type="primary" @click="save">Speichern</NButton>
        <NButton size="small" @click="cancel">Abbrechen</NButton>
      </NSpace>
    </div>

    <NGrid :cols="2" :x-gap="16" :y-gap="4">
      <NGridItem :span="2">
        <NFormItem label="Titel">
          <NInput v-if="editing" v-model:value="draft.title" />
          <span v-else>{{ listing.title }}</span>
        </NFormItem>
      </NGridItem>
      <NGridItem :span="2">
        <NFormItem label="Quell-URL">
          <NInput v-if="editing" v-model:value="draft.source_url" />
          <a
            v-else-if="listing.source_url"
            class="listing-form-source-link"
            :href="listing.source_url"
            target="_blank"
            rel="noopener"
          >
            {{ listing.source_url }}
          </a>
          <span v-else>—</span>
        </NFormItem>
      </NGridItem>

      <NGridItem>
        <NFormItem label="Preis">
          <NInputNumber v-if="editing" v-model:value="draft.price" />
          <span v-else>{{ listing.price?.toLocaleString('de-DE') ?? '—' }}</span>
        </NFormItem>
      </NGridItem>
      <NGridItem>
        <NFormItem label="Währung">
          <NInput v-if="editing" v-model:value="draft.price_currency" />
          <span v-else>{{ listing.price_currency ?? '—' }}</span>
        </NFormItem>
      </NGridItem>

      <NGridItem>
        <NFormItem label="Wohnfläche (m²)">
          <NInputNumber v-if="editing" v-model:value="draft.property_size_m2" />
          <span v-else>{{ listing.property_size_m2?.toLocaleString('de-DE') ?? '—' }}</span>
        </NFormItem>
      </NGridItem>
      <NGridItem>
        <NFormItem label="Grundstück (m²)">
          <NInputNumber v-if="editing" v-model:value="draft.plot_size_m2" />
          <span v-else>{{ listing.plot_size_m2?.toLocaleString('de-DE') ?? '—' }}</span>
        </NFormItem>
      </NGridItem>

      <NGridItem :span="2">
        <NFormItem label="Adresse">
          <NInput v-if="editing" v-model:value="draft.address" />
          <span v-else>{{ listing.address ?? '—' }}</span>
        </NFormItem>
      </NGridItem>
      <NGridItem>
        <NFormItem label="Region">
          <NInput v-if="editing" v-model:value="draft.region" />
          <span v-else>{{ listing.region ?? '—' }}</span>
        </NFormItem>
      </NGridItem>
      <NGridItem>
        <NFormItem label="Land">
          <NInput v-if="editing" v-model:value="draft.country" />
          <span v-else>{{ listing.country ?? '—' }}</span>
        </NFormItem>
      </NGridItem>

      <NGridItem>
        <NFormItem label="Breitengrad">
          <NInputNumber v-if="editing" v-model:value="draft.latitude" />
          <span v-else>{{ listing.latitude ?? '—' }}</span>
        </NFormItem>
      </NGridItem>
      <NGridItem>
        <NFormItem label="Längengrad">
          <NInputNumber v-if="editing" v-model:value="draft.longitude" />
          <span v-else>{{ listing.longitude ?? '—' }}</span>
        </NFormItem>
      </NGridItem>

      <NGridItem :span="2">
        <NFormItem label="Beschreibung">
          <NInput
            v-if="editing"
            v-model:value="draft.description"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 6 }"
          />
          <p v-else class="listing-form-text">{{ listing.description ?? '—' }}</p>
        </NFormItem>
      </NGridItem>

      <NGridItem>
        <NFormItem label="Verkäufer">
          <NInput v-if="editing" v-model:value="draft.seller_name" />
          <span v-else>{{ listing.seller_name ?? '—' }}</span>
        </NFormItem>
      </NGridItem>
      <NGridItem>
        <NFormItem label="Telefon">
          <NInput v-if="editing" v-model:value="draft.seller_phone" />
          <span v-else>{{ listing.seller_phone ?? '—' }}</span>
        </NFormItem>
      </NGridItem>
      <NGridItem>
        <NFormItem label="E-Mail">
          <NInput v-if="editing" v-model:value="draft.seller_email" />
          <span v-else>{{ listing.seller_email ?? '—' }}</span>
        </NFormItem>
      </NGridItem>
      <NGridItem>
        <NFormItem label="Letzter Kontakt">
          <NDatePicker
            v-if="editing"
            v-model:formatted-value="draft.last_contact_at"
            value-format="yyyy-MM-dd'T'HH:mm:ss'Z'"
            type="date"
          />
          <span v-else>
            {{
              listing.last_contact_at
                ? new Date(listing.last_contact_at).toLocaleDateString('de-DE')
                : '—'
            }}
          </span>
        </NFormItem>
      </NGridItem>
      <NGridItem :span="2">
        <NFormItem label="Notizen zum Verkäufer">
          <NInput
            v-if="editing"
            v-model:value="draft.seller_notes"
            type="textarea"
            :autosize="{ minRows: 1, maxRows: 4 }"
          />
          <p v-else class="listing-form-text">{{ listing.seller_notes ?? '—' }}</p>
        </NFormItem>
      </NGridItem>
    </NGrid>
  </div>
</template>

<style scoped>
.listing-form-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 0.5rem;
}

.listing-form-text {
  margin: 0;
  white-space: pre-wrap;
}

/* The browser's default link blue is too dark on the dark theme. Styled against
   prefers-color-scheme (what usePreferredDark.ts keys off), like ListingMap.vue's popup. */
.listing-form-source-link {
  color: #1a5fb4;
  overflow-wrap: anywhere;
}

@media (prefers-color-scheme: dark) {
  .listing-form-source-link {
    color: #7ab7ff;
  }
}
</style>
