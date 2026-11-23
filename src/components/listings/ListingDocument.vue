<script setup lang="ts">
import DOMPurify from 'dompurify';
import { marked } from 'marked';
import { NButton, NEmpty, NInput } from 'naive-ui';
import { computed, shallowRef } from 'vue';

/**
 * Drops the Agent-written "Bewertungstabelle" section (heading up to the next heading of the same
 * or a higher level) from the rendered view. The structured grade grid (`ListingGradeGrid.vue`) is
 * the source of truth since 01-10; the embedded section is only a stale placeholder or a copy of it.
 * Display only: the stored `document_md` and the edit view stay untouched. See docs/buglist.md.
 */
function stripGradeTableSection(md: string): string {
  const kept: string[] = [];
  let skipLevel = 0;
  for (const line of md.split('\n')) {
    const heading = /^(#{1,6})\s+(.*?)\s*#*\s*$/.exec(line);
    if (heading) {
      const level = heading[1].length;
      if (skipLevel > 0 && level <= skipLevel) {
        skipLevel = 0;
      }
      if (skipLevel === 0 && /^bewertungstabelle\b/i.test(heading[2])) {
        skipLevel = level;
      }
    }
    if (skipLevel === 0) {
      kept.push(line);
    }
  }
  return kept.join('\n');
}

const props = defineProps<{
  documentMd?: string;
}>();

const emit = defineEmits<{
  save: [documentMd: string];
}>();

const editing = shallowRef(false);
const draft = shallowRef(props.documentMd ?? '');

const renderedHtml = computed(() => {
  if (!props.documentMd) {
    return '';
  }
  return DOMPurify.sanitize(
    marked.parse(stripGradeTableSection(props.documentMd), { async: false }),
  );
});

function startEdit() {
  draft.value = props.documentMd ?? '';
  editing.value = true;
}

function save() {
  editing.value = false;
  if (draft.value !== (props.documentMd ?? '')) {
    emit('save', draft.value);
  }
}
</script>

<template>
  <div class="listing-document">
    <div class="listing-document-header">
      <h3>Listing Document</h3>
      <NButton v-if="!editing" size="small" quaternary @click="startEdit">Bearbeiten</NButton>
    </div>

    <NInput
      v-if="editing"
      v-model:value="draft"
      type="textarea"
      :autosize="{ minRows: 6, maxRows: 30 }"
      @blur="save"
    />
    <div v-else-if="documentMd" class="listing-document-rendered" v-html="renderedHtml" />
    <NEmpty v-else description="Noch kein Listing Document vorhanden" />
  </div>
</template>

<style scoped>
.listing-document-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.listing-document-rendered {
  line-height: 1.5;
}
</style>
