<script setup lang="ts">
import { NEmpty } from 'naive-ui';
import { onBeforeUnmount, onMounted, useTemplateRef } from 'vue';

import ListingDetailPanel from '@/components/listings/ListingDetailPanel.vue';

// Shared master-detail split layout: left slot content (a table, a map, ...)
// stays visible while the selected Listing's full ListingDetailPanel renders
// on the right. Extracted out of ListingsPanel.vue's "Liste" tab once Map
// View (spec 01-06) needed the identical pattern — same scroll behavior,
// same :key-forced-remount fix, same empty state.
//
// Layout: the split is a fixed-height shell that fills the viewport from its
// own top edge down, and both columns scroll independently inside it. An
// earlier version made the right pane `position: sticky`; that only holds
// while the pane stays inside its containing block, which was barely taller
// than the pane itself, so near the bottom the pane scrolled away with the
// page (docs/buglist.md, issue #2). A shell that fits the viewport removes
// the page scroll the pane could leak into. CSS alone cannot know the shell's
// offset from the top of the page (tabs, banner, heading above it), so it is
// measured here and exposed as `--listing-split-top`.
defineProps<{
  selectedListingId: string | null;
}>();

const root = useTemplateRef<HTMLElement>('root');

let resizeObserver: ResizeObserver | null = null;

function updateOffset() {
  const el = root.value;
  if (!el) return;
  // A hidden tab pane (display: none) reports a zero rect; ignore it. The
  // observer fires again once the pane becomes visible.
  if (el.offsetParent === null) return;
  const top = el.getBoundingClientRect().top + window.scrollY;
  el.style.setProperty('--listing-split-top', `${Math.round(top)}px`);
}

onMounted(() => {
  updateOffset();
  // The element itself changes size when its tab pane is shown; the body
  // changes size when content above the split (banner, heading) appears.
  resizeObserver = new ResizeObserver(updateOffset);
  if (root.value) resizeObserver.observe(root.value);
  resizeObserver.observe(document.body);
});

onBeforeUnmount(() => resizeObserver?.disconnect());
</script>

<template>
  <div ref="root" class="listing-split-pane">
    <div class="listing-split-pane-left">
      <slot />
    </div>
    <div class="listing-split-pane-right">
      <ListingDetailPanel
        v-if="selectedListingId"
        :key="selectedListingId"
        :listing-id="selectedListingId"
      />
      <NEmpty v-else description="Listing auswählen" class="listing-split-pane-empty" />
    </div>
  </div>
</template>

<style scoped>
.listing-split-pane {
  --listing-split-top: 0px;
  display: flex;
  gap: 1.5rem;
  align-items: stretch;
  /* Fill the viewport below the split's own top edge, minus the shell's bottom padding. */
  height: calc(100vh - var(--listing-split-top) - 2rem);
  min-height: 400px;
}

.listing-split-pane-left {
  flex: 1 1 55%;
  min-width: 0;
  overflow-y: auto;
}

.listing-split-pane-right {
  flex: 1 1 45%;
  min-width: 0;
  overflow-y: auto;
  /* Stop the page from taking over once the pane reaches its end. */
  overscroll-behavior: contain;
}

.listing-split-pane-empty {
  margin-top: 3rem;
}
</style>
