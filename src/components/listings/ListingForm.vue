<script setup lang="ts">
import { NDatePicker, NFormItem, NGrid, NGridItem, NInput, NInputNumber, NSelect } from 'naive-ui';
import { reactive, watch } from 'vue';

import type { ListingInput } from '@/api/listings';
import type { Listing, ListingStatus } from '@/types/listing';

const props = defineProps<{
  listing: Listing;
}>();

const emit = defineEmits<{
  save: [patch: ListingInput];
}>();

const statusOptions: { label: string; value: ListingStatus }[] = [
  { label: 'Neu', value: 'new' },
  { label: 'Wartend', value: 'waiting' },
  { label: 'In Verhandlung', value: 'in_negotiation' },
  { label: 'Besichtigt', value: 'visited' },
  { label: 'Abgelehnt', value: 'rejected' },
  { label: 'Archiviert', value: 'archived' },
];

// Local editable copy — the listing prop stays the source of truth; a save
// only fires when a field's value actually differs from it (spec 01-07:
// "any field edit saves immediately", but a no-op blur shouldn't spam
// user_confirmed_fields with unchanged fields).
const draft = reactive({ ...props.listing });

watch(
  () => props.listing,
  (updated) => Object.assign(draft, updated),
);

function saveIfChanged<K extends keyof ListingInput>(field: K) {
  const value = draft[field as keyof typeof draft] as ListingInput[K];
  if (value !== props.listing[field as keyof Listing]) {
    emit('save', { [field]: value } as ListingInput);
  }
}

function saveStatus() {
  // status/rejection_comment travel together — the backend validates
  // "rejected requires a comment" against the merged pair.
  emit('save', { status: draft.status, rejection_comment: draft.rejection_comment });
}
</script>

<template>
  <NGrid :cols="2" :x-gap="16" :y-gap="4">
    <NGridItem :span="2">
      <NFormItem label="Titel">
        <NInput v-model:value="draft.title" @blur="saveIfChanged('title')" />
      </NFormItem>
    </NGridItem>
    <NGridItem :span="2">
      <NFormItem label="Quell-URL">
        <NInput v-model:value="draft.source_url" @blur="saveIfChanged('source_url')" />
      </NFormItem>
    </NGridItem>

    <NGridItem>
      <NFormItem label="Preis">
        <NInputNumber v-model:value="draft.price" @blur="saveIfChanged('price')" />
      </NFormItem>
    </NGridItem>
    <NGridItem>
      <NFormItem label="Währung">
        <NInput v-model:value="draft.price_currency" @blur="saveIfChanged('price_currency')" />
      </NFormItem>
    </NGridItem>

    <NGridItem>
      <NFormItem label="Wohnfläche (m²)">
        <NInputNumber
          v-model:value="draft.property_size_m2"
          @blur="saveIfChanged('property_size_m2')"
        />
      </NFormItem>
    </NGridItem>
    <NGridItem>
      <NFormItem label="Grundstück (m²)">
        <NInputNumber v-model:value="draft.plot_size_m2" @blur="saveIfChanged('plot_size_m2')" />
      </NFormItem>
    </NGridItem>

    <NGridItem :span="2">
      <NFormItem label="Adresse">
        <NInput v-model:value="draft.address" @blur="saveIfChanged('address')" />
      </NFormItem>
    </NGridItem>
    <NGridItem>
      <NFormItem label="Region">
        <NInput v-model:value="draft.region" @blur="saveIfChanged('region')" />
      </NFormItem>
    </NGridItem>
    <NGridItem>
      <NFormItem label="Land">
        <NInput v-model:value="draft.country" @blur="saveIfChanged('country')" />
      </NFormItem>
    </NGridItem>

    <NGridItem>
      <NFormItem label="Breitengrad">
        <NInputNumber v-model:value="draft.latitude" @blur="saveIfChanged('latitude')" />
      </NFormItem>
    </NGridItem>
    <NGridItem>
      <NFormItem label="Längengrad">
        <NInputNumber v-model:value="draft.longitude" @blur="saveIfChanged('longitude')" />
      </NFormItem>
    </NGridItem>

    <NGridItem :span="2">
      <NFormItem label="Beschreibung">
        <NInput
          v-model:value="draft.description"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 6 }"
          @blur="saveIfChanged('description')"
        />
      </NFormItem>
    </NGridItem>

    <NGridItem>
      <NFormItem label="Status">
        <NSelect v-model:value="draft.status" :options="statusOptions" @update:value="saveStatus" />
      </NFormItem>
    </NGridItem>
    <NGridItem v-if="draft.status === 'rejected'">
      <NFormItem label="Ablehnungsgrund">
        <NInput v-model:value="draft.rejection_comment" @blur="saveStatus" />
      </NFormItem>
    </NGridItem>

    <NGridItem>
      <NFormItem label="Verkäufer">
        <NInput v-model:value="draft.seller_name" @blur="saveIfChanged('seller_name')" />
      </NFormItem>
    </NGridItem>
    <NGridItem>
      <NFormItem label="Telefon">
        <NInput v-model:value="draft.seller_phone" @blur="saveIfChanged('seller_phone')" />
      </NFormItem>
    </NGridItem>
    <NGridItem>
      <NFormItem label="E-Mail">
        <NInput v-model:value="draft.seller_email" @blur="saveIfChanged('seller_email')" />
      </NFormItem>
    </NGridItem>
    <NGridItem>
      <NFormItem label="Letzter Kontakt">
        <NDatePicker
          v-model:formatted-value="draft.last_contact_at"
          value-format="yyyy-MM-dd'T'HH:mm:ss'Z'"
          type="date"
          @update:formatted-value="saveIfChanged('last_contact_at')"
        />
      </NFormItem>
    </NGridItem>
    <NGridItem :span="2">
      <NFormItem label="Notizen zum Verkäufer">
        <NInput
          v-model:value="draft.seller_notes"
          type="textarea"
          :autosize="{ minRows: 1, maxRows: 4 }"
          @blur="saveIfChanged('seller_notes')"
        />
      </NFormItem>
    </NGridItem>
  </NGrid>
</template>
