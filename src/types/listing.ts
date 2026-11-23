/** Mirrors backend/internal/domain.Listing JSON exactly (snake_case). */
export type ListingStatus =
  | 'new'
  | 'waiting'
  | 'in_negotiation'
  | 'visited'
  | 'rejected'
  | 'archived';

export interface Listing {
  id: string;
  profile_id: string;
  title: string;
  source_url?: string;
  price?: number;
  price_currency?: string;
  property_size_m2?: number;
  plot_size_m2?: number;
  address?: string;
  region?: string;
  country?: string;
  latitude?: number;
  longitude?: number;
  description?: string;
  title_image_path?: string;
  title_image_id?: string;
  status: ListingStatus;
  rejection_comment?: string;
  overall_score?: number;
  seller_name?: string;
  seller_phone?: string;
  seller_email?: string;
  seller_notes?: string;
  last_contact_at?: string;
  document_md?: string;
  source_type: 'url' | 'pdf' | 'image' | 'manual';
  source_file_path?: string;
  user_confirmed_fields: readonly string[];
  needs_reevaluation: boolean;
  kurzfazit?: string;
  created_at: string;
  updated_at: string;
}

/** Mirrors backend/internal/domain.ListingImage JSON. */
export interface ListingImage {
  id: string;
  listing_id: string;
  url: string;
  is_local: boolean;
  sort_order: number;
  file_path: string;
  category?: string;
  created_at: string;
}

/** Mirrors backend/internal/domain.ListingNote JSON. */
export interface ListingNote {
  id: string;
  listing_id: string;
  content: string;
  created_at: string;
  updated_at: string;
}

/** Mirrors backend/internal/domain.CriterionScore JSON (evaluate_listing response). */
export interface CriterionGradeResult {
  criterion_id: string;
  grade: 'A' | 'B' | 'C' | 'D' | 'E' | 'F';
  reasoning: string;
}

/** Mirrors backend/internal/domain.DuplicateListing JSON (409 body of a possible duplicate). */
export interface DuplicateListing {
  id: string;
  title: string;
  address?: string;
  matches: ('address' | 'coordinates')[];
}
