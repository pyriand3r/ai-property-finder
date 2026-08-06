import type { ListingGrade } from '@/types/grade';

/** Shared grade -> color mapping (ListingGradeGrid's Bewertung column, List
 * View's optional score columns) — A/B green through F dark red. */
export const gradeColor: Record<ListingGrade['grade'], string> = {
  A: '#18a058',
  B: '#36ad6a',
  C: '#f0a020',
  D: '#e88080',
  E: '#d03050',
  F: '#a01030',
};
