<script setup lang="ts">
import { NAlert, NEmpty, NList, NListItem, NSpin, useMessage } from 'naive-ui';
import { onMounted, shallowRef } from 'vue';

import CriterionCreateForm from '@/components/criteria/CriterionCreateForm.vue';
import CriterionListItem from '@/components/criteria/CriterionListItem.vue';
import ReevaluationBanner from '@/components/shared/ReevaluationBanner.vue';
import { useCriteria } from '@/composables/useCriteria';

const props = defineProps<{
  profileId: string;
}>();

const message = useMessage();
const {
  criteria,
  loading,
  error,
  fetchCriteria,
  createCriterion,
  updateCriterion,
  deleteCriterion,
} = useCriteria(props.profileId);

const creating = shallowRef(false);

onMounted(fetchCriteria);

async function handleCreate(
  name: string,
  description: string,
  weight: number,
  isSubjective: boolean,
) {
  creating.value = true;
  try {
    await createCriterion({ name, description, weight, is_subjective: isSubjective });
  } catch (createError) {
    message.error(createError instanceof Error ? createError.message : String(createError));
  } finally {
    creating.value = false;
  }
}

async function handleUpdateWeight(criterionId: string, weight: number) {
  try {
    await updateCriterion(criterionId, { weight });
  } catch (updateError) {
    message.error(updateError instanceof Error ? updateError.message : String(updateError));
  }
}

async function handleUpdateSubjective(criterionId: string, isSubjective: boolean) {
  try {
    await updateCriterion(criterionId, { is_subjective: isSubjective });
  } catch (updateError) {
    message.error(updateError instanceof Error ? updateError.message : String(updateError));
  }
}

async function handleUpdateText(criterionId: string, name: string, description: string) {
  try {
    await updateCriterion(criterionId, { name, description });
  } catch (updateError) {
    message.error(updateError instanceof Error ? updateError.message : String(updateError));
  }
}

async function handleDelete(criterionId: string) {
  try {
    await deleteCriterion(criterionId);
  } catch (deleteError) {
    message.error(deleteError instanceof Error ? deleteError.message : String(deleteError));
  }
}
</script>

<template>
  <div class="criteria-editor">
    <ReevaluationBanner :profile-id="profileId" />

    <CriterionCreateForm :creating="creating" @create="handleCreate" />

    <NAlert v-if="error" type="error" :title="error" class="criteria-editor-error" />

    <NSpin :show="loading">
      <NEmpty
        v-if="criteria.length === 0"
        description="Noch keine Kriterien — leg oben eines an."
      />
      <NList v-else bordered class="criteria-editor-list">
        <NListItem v-for="criterion in criteria" :key="criterion.id">
          <CriterionListItem
            :criterion="criterion"
            @update-weight="handleUpdateWeight"
            @update-subjective="handleUpdateSubjective"
            @update-text="handleUpdateText"
            @delete="handleDelete"
          />
        </NListItem>
      </NList>
    </NSpin>
  </div>
</template>

<style scoped>
.criteria-editor-error {
  margin-block: 1rem;
}

.criteria-editor-list {
  margin-top: 1rem;
}
</style>
