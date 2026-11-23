<script setup lang="ts">
import {
  darkTheme,
  NButton,
  NConfigProvider,
  NDialogProvider,
  NGlobalStyle,
  NMessageProvider,
} from 'naive-ui';
import { useRouter } from 'vue-router';

import { useAuth } from '@/composables/useAuth';
import { usePreferredDark } from '@/composables/usePreferredDark';

const prefersDark = usePreferredDark();
const { isAuthenticated, logout } = useAuth();
const router = useRouter();

async function handleLogout() {
  await logout();
  await router.push({ name: 'login' });
}
</script>

<template>
  <NConfigProvider :theme="prefersDark ? darkTheme : null">
    <NGlobalStyle />
    <NMessageProvider>
      <NDialogProvider>
        <NButton v-if="isAuthenticated" class="app-logout" text @click="handleLogout"
          >Abmelden</NButton
        >
        <RouterView v-slot="{ Component, route }">
          <component :is="Component" :key="route.fullPath" />
        </RouterView>
      </NDialogProvider>
    </NMessageProvider>
  </NConfigProvider>
</template>

<style scoped>
.app-logout {
  position: fixed;
  top: 0.5rem;
  right: 1rem;
  z-index: 1000;
}
</style>
