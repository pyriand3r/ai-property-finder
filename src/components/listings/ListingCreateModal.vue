<script setup lang="ts">
import {
  NAlert,
  NButton,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
  NSpace,
  useMessage,
} from 'naive-ui';
import { reactive, shallowRef } from 'vue';

import { ApiError } from '@/api/http';
import { createListing } from '@/api/listings';

import type { ListingInput } from '@/api/listings';
import type { DuplicateListing } from '@/types/listing';

const props = defineProps<{
  profileId: string;
}>();

const show = defineModel<boolean>('show', { required: true });

const emit = defineEmits<{
  created: [listingId: string];
}>();

const message = useMessage();

const draft = reactive({
  title: '',
  source_url: '',
  address: '',
  region: '',
  country: '',
  latitude: null as number | null,
  longitude: null as number | null,
  price: null as number | null,
});

const saving = shallowRef(false);
const error = shallowRef<string | null>(null);
/** Set when the backend warned about a possible duplicate (issue #10); the user may confirm. */
const duplicates = shallowRef<DuplicateListing[]>([]);

function buildInput(): ListingInput {
  const input: ListingInput = { title: draft.title.trim() };
  for (const field of ['source_url', 'address', 'region', 'country'] as const) {
    const value = draft[field].trim();
    if (value) input[field] = value;
  }
  for (const field of ['latitude', 'longitude', 'price'] as const) {
    if (draft[field] !== null) input[field] = draft[field];
  }
  return input;
}

function matchLabel(duplicate: DuplicateListing): string {
  return duplicate.matches
    .map((match) => (match === 'address' ? 'gleiche Adresse' : 'gleiche Koordinaten'))
    .join(', ');
}

function reset() {
  Object.assign(draft, {
    title: '',
    source_url: '',
    address: '',
    region: '',
    country: '',
    latitude: null,
    longitude: null,
    price: null,
  });
  error.value = null;
  duplicates.value = [];
}

/**
 * Backend: POST .../listings. A 409 carrying `duplicates` is a warning, not a failure: it is
 * shown above the buttons, and "Trotzdem anlegen" repeats the call with confirm_duplicate=true.
 */
async function submit(confirmDuplicate: boolean) {
  if (!draft.title.trim() || saving.value) return;
  saving.value = true;
  error.value = null;
  try {
    const listing = await createListing(props.profileId, buildInput(), confirmDuplicate);
    message.success('Inserat angelegt');
    show.value = false;
    reset();
    emit('created', listing.id);
  } catch (createError) {
    if (createError instanceof ApiError && createError.duplicates?.length) {
      duplicates.value = createError.duplicates;
    } else {
      duplicates.value = [];
      error.value = createError instanceof Error ? createError.message : String(createError);
    }
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <NModal
    v-model:show="show"
    preset="card"
    title="Inserat anlegen"
    class="listing-create-modal"
    :mask-closable="!saving"
  >
    <form @submit.prevent="submit(false)">
      <NFormItem label="Titel" required>
        <NInput v-model:value="draft.title" />
      </NFormItem>
      <NFormItem label="Quell-URL">
        <NInput v-model:value="draft.source_url" />
      </NFormItem>
      <NFormItem label="Adresse">
        <NInput v-model:value="draft.address" />
      </NFormItem>
      <NFormItem label="Region">
        <NInput v-model:value="draft.region" />
      </NFormItem>
      <NFormItem label="Land">
        <NInput v-model:value="draft.country" />
      </NFormItem>
      <NFormItem label="Breitengrad">
        <NInputNumber v-model:value="draft.latitude" :show-button="false" />
      </NFormItem>
      <NFormItem label="Längengrad">
        <NInputNumber v-model:value="draft.longitude" :show-button="false" />
      </NFormItem>
      <NFormItem label="Preis">
        <NInputNumber v-model:value="draft.price" :show-button="false" />
      </NFormItem>

      <NAlert v-if="error" type="error" :title="error" class="listing-create-modal-alert" />
      <NAlert
        v-if="duplicates.length"
        type="warning"
        title="Mögliches Duplikat"
        class="listing-create-modal-alert"
      >
        In diesem Profil existiert bereits ein Inserat mit gleicher Adresse oder gleichen
        Koordinaten:
        <ul class="listing-create-modal-duplicates">
          <li v-for="duplicate in duplicates" :key="duplicate.id">
            {{ duplicate.title }} ({{ matchLabel(duplicate) }})
          </li>
        </ul>
      </NAlert>

      <NSpace justify="end">
        <NButton :disabled="saving" @click="show = false">Abbrechen</NButton>
        <NButton v-if="duplicates.length" type="warning" :loading="saving" @click="submit(true)">
          Trotzdem anlegen
        </NButton>
        <NButton
          v-else
          type="primary"
          attr-type="submit"
          :disabled="!draft.title.trim()"
          :loading="saving"
        >
          Anlegen
        </NButton>
      </NSpace>
    </form>
  </NModal>
</template>

<style scoped>
.listing-create-modal {
  width: min(32rem, 92vw);
}

.listing-create-modal-alert {
  margin-bottom: 1rem;
}

.listing-create-modal-duplicates {
  margin: 0.25rem 0 0;
  padding-left: 1.25rem;
}
</style>
