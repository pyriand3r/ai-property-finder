<script setup lang="ts">
import { NButton, NInput } from 'naive-ui';
import { shallowRef } from 'vue';

const props = defineProps<{
  /** True while the parent's create request is in flight. */
  creating: boolean;
}>();

const emit = defineEmits<{
  create: [name: string];
}>();

const name = shallowRef('');

function submit() {
  const trimmed = name.value.trim();
  if (!trimmed || props.creating) {
    return;
  }
  emit('create', trimmed);
  name.value = '';
}
</script>

<template>
  <form class="profile-create-form" @submit.prevent="submit">
    <NInput v-model:value="name" placeholder="Name des Profils (z. B. Schweden)" clearable />
    <NButton type="primary" attr-type="submit" :disabled="!name.trim()" :loading="creating">
      Profil anlegen
    </NButton>
  </form>
</template>

<style scoped>
.profile-create-form {
  display: flex;
  gap: 0.5rem;
}
</style>
