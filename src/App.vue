<script setup lang="ts">
import { darkTheme, NConfigProvider, NDialogProvider, NMessageProvider } from 'naive-ui';

import { usePreferredDark } from '@/composables/usePreferredDark';

const prefersDark = usePreferredDark();
</script>

<template>
  <NConfigProvider :theme="prefersDark ? darkTheme : null">
    <NMessageProvider>
      <NDialogProvider>
        <RouterView />
      </NDialogProvider>
    </NMessageProvider>
  </NConfigProvider>
</template>
