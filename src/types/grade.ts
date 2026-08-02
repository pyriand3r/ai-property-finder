/** Mirrors backend/internal/domain.ListingGrade JSON (GET
 * /api/listings/{id}/grades — spec 01-07 Layer 5.3 grade grid). */
export interface ListingGrade {
  criterion_id: string;
  criterion_name: string;
  grade: 'A' | 'B' | 'C' | 'D' | 'E' | 'F';
  reasoning: string;
}
