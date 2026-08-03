<script setup lang="ts">
import { NProgress, NTooltip } from 'naive-ui';
import { computed } from 'vue';

// Claude's context window (spec 01-01, ADR 0014). Hardcoded here rather
// than sourced from app_config — prop-finder's llm.* config keys don't
// carry a context-window size (see 00-03-llm-agent-integration "Config"),
// and this donut is purely informational either way.
const CONTEXT_WINDOW_TOKENS = 200_000;

const props = defineProps<{
  tokenCountEstimate?: number;
}>();

const percentage = computed(() =>
  Math.min(100, Math.round(((props.tokenCountEstimate ?? 0) / CONTEXT_WINDOW_TOKENS) * 100)),
);

const status = computed(() => {
  if (percentage.value >= 80) return 'error';
  if (percentage.value >= 60) return 'warning';
  return 'success';
});
</script>

<template>
  <NTooltip trigger="hover">
    <template #trigger>
      <NProgress
        type="circle"
        :percentage="percentage"
        :status="status"
        :stroke-width="14"
        style="width: 40px"
      />
    </template>
    {{ tokenCountEstimate ?? 0 }} / {{ CONTEXT_WINDOW_TOKENS }} Tokens
  </NTooltip>
</template>
