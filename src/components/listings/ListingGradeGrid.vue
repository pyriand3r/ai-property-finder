<script setup lang="ts">
import { NEmpty, NTable, NTag } from 'naive-ui';

import { gradeColor } from '@/utils/gradeColor';

import type { ListingGrade } from '@/types/grade';

defineProps<{
  grades: readonly ListingGrade[];
}>();
</script>

<template>
  <NEmpty v-if="grades.length === 0" description="Noch nicht bewertet" />
  <NTable v-else :bordered="false" :single-line="false">
    <thead>
      <tr>
        <th>Kriterium</th>
        <th>Note</th>
        <th>Begründung</th>
      </tr>
    </thead>
    <tbody>
      <tr v-for="grade in grades" :key="grade.criterion_id">
        <td>{{ grade.criterion_name }}</td>
        <td>
          <NTag :color="{ color: gradeColor[grade.grade], textColor: 'white' }" round>
            {{ grade.grade }}
          </NTag>
        </td>
        <td>{{ grade.reasoning }}</td>
      </tr>
    </tbody>
  </NTable>
</template>
