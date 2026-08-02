<script setup lang="ts">
import { NButton, NInput, NPopconfirm, NSlider, NSpace, NSwitch, NText } from 'naive-ui';
import { shallowRef } from 'vue';

import type { Criterion } from '@/types/criterion';

const props = defineProps<{
  criterion: Criterion;
}>();

const emit = defineEmits<{
  'update-weight': [criterionId: string, weight: number];
  'update-subjective': [criterionId: string, isSubjective: boolean];
  'update-text': [criterionId: string, name: string, description: string];
  delete: [criterionId: string];
}>();

const editing = shallowRef(false);
const draftName = shallowRef(props.criterion.name);
const draftDescription = shallowRef(props.criterion.description);

function startEdit() {
  draftName.value = props.criterion.name;
  draftDescription.value = props.criterion.description;
  editing.value = true;
}

function confirmEdit() {
  editing.value = false;
  const name = draftName.value.trim();
  const description = draftDescription.value.trim();
  if (name && (name !== props.criterion.name || description !== props.criterion.description)) {
    emit('update-text', props.criterion.id, name, description);
  }
}
</script>

<template>
  <div class="criterion-item">
    <div v-if="!editing" class="criterion-item-text" @click="startEdit">
      <NText strong>{{ criterion.name }}</NText>
      <NText depth="3" class="criterion-item-description">{{ criterion.description }}</NText>
    </div>
    <div v-else class="criterion-item-edit">
      <NInput v-model:value="draftName" placeholder="Name" @keyup.enter="confirmEdit" />
      <NInput
        v-model:value="draftDescription"
        type="textarea"
        placeholder="Beschreibung"
        :autosize="{ minRows: 1, maxRows: 4 }"
        @blur="confirmEdit"
      />
    </div>

    <NSpace class="criterion-item-controls" align="center">
      <span class="criterion-item-label">Subjektiv</span>
      <NSwitch
        :value="criterion.is_subjective"
        @update:value="emit('update-subjective', criterion.id, $event)"
      />
      <span class="criterion-item-label">Gewichtung</span>
      <NSlider
        class="criterion-item-slider"
        :value="criterion.weight"
        :min="1"
        :max="5"
        :step="1"
        :marks="{ 1: '1', 2: '2', 3: '3', 4: '4', 5: '5' }"
        @update:value="emit('update-weight', criterion.id, $event)"
      />
      <NPopconfirm @positive-click="emit('delete', criterion.id)">
        <template #trigger>
          <NButton size="small" quaternary type="error">Löschen</NButton>
        </template>
        Kriterium "{{ criterion.name }}" wirklich löschen?
      </NPopconfirm>
    </NSpace>
  </div>
</template>

<style scoped>
.criterion-item {
  padding: 0.75rem 0;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.criterion-item-text {
  cursor: pointer;
  display: flex;
  flex-direction: column;
}

.criterion-item-description {
  font-size: 0.85rem;
}

.criterion-item-edit {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.criterion-item-controls {
  flex-wrap: nowrap;
}

.criterion-item-label {
  font-size: 0.8rem;
  color: var(--n-text-color-3, #888);
  white-space: nowrap;
}

.criterion-item-slider {
  width: 160px;
}
</style>
