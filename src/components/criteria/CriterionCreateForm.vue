<script setup lang="ts">
import { NButton, NInput, NSlider, NSpace, NSwitch } from 'naive-ui';
import { shallowRef } from 'vue';

const props = defineProps<{
  creating: boolean;
}>();

const emit = defineEmits<{
  create: [name: string, description: string, weight: number, isSubjective: boolean];
}>();

const name = shallowRef('');
const description = shallowRef('');
const weight = shallowRef(3);
const isSubjective = shallowRef(false);

function submit() {
  const trimmedName = name.value.trim();
  if (!trimmedName || props.creating) {
    return;
  }
  emit('create', trimmedName, description.value.trim(), weight.value, isSubjective.value);
  name.value = '';
  description.value = '';
  weight.value = 3;
  isSubjective.value = false;
}
</script>

<template>
  <form class="criterion-create-form" @submit.prevent="submit">
    <NSpace vertical>
      <NInput v-model:value="name" placeholder="Kriterium (z. B. Nähe zum Wasser)" />
      <NInput
        v-model:value="description"
        type="textarea"
        placeholder="Beschreibung — wird zur Bewertung an das Modell gegeben"
        :autosize="{ minRows: 1, maxRows: 3 }"
      />
      <NSpace align="center">
        <span>Subjektiv</span>
        <NSwitch v-model:value="isSubjective" />
        <span>Gewichtung {{ weight }}</span>
        <NSlider v-model:value="weight" :min="1" :max="5" :step="1" style="width: 160px" />
        <NButton type="primary" attr-type="submit" :disabled="!name.trim()" :loading="creating">
          Kriterium hinzufügen
        </NButton>
      </NSpace>
    </NSpace>
  </form>
</template>
