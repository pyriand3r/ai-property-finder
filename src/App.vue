<script setup lang="ts">
import {
  darkTheme,
  NConfigProvider,
  NDialogProvider,
  NGlobalStyle,
  NMessageProvider,
} from 'naive-ui';

import { usePreferredDark } from '@/composables/usePreferredDark';

const prefersDark = usePreferredDark();
</script>

<template>
  <NConfigProvider :theme="prefersDark ? darkTheme : null">
    <NGlobalStyle />
    <NMessageProvider>
      <NDialogProvider>
        <RouterView v-slot="{ Component, route }">
          <component :is="Component" :key="route.fullPath" />
        </RouterView>
      </NDialogProvider>
    </NMessageProvider>
  </NConfigProvider>
</template>
