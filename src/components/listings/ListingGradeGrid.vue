<script setup lang="ts">
import { NEmpty, NTable, NTag } from 'naive-ui';

import type { ListingGrade } from '@/types/grade';

defineProps<{
  grades: readonly ListingGrade[];
}>();

const gradeColor: Record<ListingGrade['grade'], string> = {
  A: '#18a058',
  B: '#36ad6a',
  C: '#f0a020',
  D: '#e88080',
  E: '#d03050',
  F: '#a01030',
};
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
