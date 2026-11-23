import { http } from '@/api/http';

import type { ListingGrade } from '@/types/grade';
import type { CriterionGradeResult, Listing } from '@/types/listing';
import type { Task } from '@/types/task';

export function listListings(profileId: string, needsReevaluation?: boolean) {
  const query = needsReevaluation === undefined ? '' : `?needs_reevaluation=${needsReevaluation}`;
  return http.get<Listing[]>(`/api/profiles/${profileId}/listings${query}`);
}

export function getListing(listingId: string) {
  return http.get<Listing>(`/api/listings/${listingId}`);
}

type ListingInputBase = Omit<
  Listing,
  | 'id'
  | 'profile_id'
  | 'overall_score'
  | 'user_confirmed_fields'
  | 'needs_reevaluation'
  | 'kurzfazit'
  | 'source_type'
  | 'source_file_path'
  | 'title_image_id'
  | 'created_at'
  | 'updated_at'
>;

/**
 * Body of POST/PATCH listing. An omitted key means "unchanged"; an explicit `null` clears the
 * field (backend `service.Optional`, issue #9). `title` and `status` are required in the
 * database and cannot be cleared.
 */
export type ListingInput = {
  [K in keyof ListingInputBase]?: K extends 'title' | 'status'
    ? ListingInputBase[K]
    : ListingInputBase[K] | null;
} & { title_image_id?: string };

export function createListing(profileId: string, input: ListingInput) {
  return http.post<Listing>(`/api/profiles/${profileId}/listings`, input);
}

export function updateListing(listingId: string, input: ListingInput) {
  return http.patch<Listing>(`/api/listings/${listingId}`, input);
}

export function deleteListing(listingId: string) {
  return http.delete<void>(`/api/listings/${listingId}`);
}

export interface EvaluateResponse {
  listing: Listing;
  grades: CriterionGradeResult[];
}

export function evaluateListing(listingId: string) {
  return http.post<EvaluateResponse>(`/api/listings/${listingId}/evaluate`);
}

export function reevaluateAll(profileId: string) {
  return http.post<Task>(`/api/profiles/${profileId}/listings/reevaluate-all`);
}

export function getListingGrades(listingId: string) {
  return http.get<ListingGrade[]>(`/api/listings/${listingId}/grades`);
}
