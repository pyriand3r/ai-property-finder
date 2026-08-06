<script setup lang="ts">
import { onMounted } from 'vue';

import {
  getProfileContext,
  getProfileContextHistory,
  restoreProfileContext,
  updateProfileContext,
} from '@/api/context';
import ContextEditor from '@/components/context/ContextEditor.vue';
import { useContextEditor } from '@/composables/useContextEditor';

const props = defineProps<{
  profileId: string;
}>();

const { context, history, loading, saving, error, fetchAll, save, restore } = useContextEditor(
  () => getProfileContext(props.profileId),
  (contentMd) => updateProfileContext(props.profileId, contentMd),
  () => getProfileContextHistory(props.profileId),
  (revisionId) => restoreProfileContext(props.profileId, revisionId),
);

onMounted(fetchAll);
</script>

<template>
  <ContextEditor
    :content="context?.content_md ?? ''"
    :history="history"
    :loading="loading"
    :saving="saving"
    :error="error"
    @save="save"
    @restore="restore"
  />
</template>
