<script setup lang="ts">
import { NButton, NCard, NInput } from 'naive-ui';
import { shallowRef } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { ApiError } from '@/api/http';
import { useAuth } from '@/composables/useAuth';

const { login } = useAuth();
const route = useRoute();
const router = useRouter();

const password = shallowRef('');
const submitting = shallowRef(false);
const error = shallowRef<string | null>(null);

async function submit() {
  if (!password.value || submitting.value) {
    return;
  }
  submitting.value = true;
  error.value = null;
  try {
    await login(password.value);
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/';
    await router.replace(redirect);
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : 'Anmeldung fehlgeschlagen';
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div class="login-view">
    <NCard title="Anmeldung" class="login-view__card">
      <form class="login-view__form" @submit.prevent="submit">
        <NInput
          v-model:value="password"
          type="password"
          placeholder="Passwort"
          show-password-on="click"
          autofocus
        />
        <p v-if="error" class="login-view__error">{{ error }}</p>
        <NButton type="primary" attr-type="submit" :disabled="!password" :loading="submitting" block>
          Anmelden
        </NButton>
      </form>
    </NCard>
  </div>
</template>

<style scoped>
.login-view {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
}
.login-view__card {
  width: 320px;
}
.login-view__form {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.login-view__error {
  color: var(--n-error-color, #d03050);
  margin: 0;
}
</style>
