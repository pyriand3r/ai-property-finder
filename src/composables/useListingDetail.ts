import { readonly, shallowRef } from 'vue';

import { evaluateListing, getListing, getListingGrades, updateListing } from '@/api/listings';

import type { ListingInput } from '@/api/listings';
import type { ListingGrade } from '@/types/grade';
import type { Listing } from '@/types/listing';

/** Loads one Listing plus its grade grid and exposes the field-save and
 * "Neu bewerten" actions the Detail View needs (spec 01-07 Layer 5.3). */
export function useListingDetail(listingId: string) {
  const listing = shallowRef<Listing | null>(null);
  const grades = shallowRef<ListingGrade[]>([]);
  const loading = shallowRef(false);
  const saving = shallowRef(false);
  const evaluating = shallowRef(false);
  const error = shallowRef<string | null>(null);

  async function fetchAll() {
    loading.value = true;
    error.value = null;
    try {
      const [fetchedListing, fetchedGrades] = await Promise.all([
        getListing(listingId),
        getListingGrades(listingId),
      ]);
      listing.value = fetchedListing;
      grades.value = fetchedGrades;
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    } finally {
      loading.value = false;
    }
  }

  /** Saves a single field (or a small related group, e.g. status +
   * rejection_comment together) immediately — matches the app's
   * low-friction "any field edit saves immediately" rule (spec 01-07). */
  async function saveField(patch: ListingInput) {
    saving.value = true;
    error.value = null;
    try {
      listing.value = await updateListing(listingId, patch);
    } catch (saveError) {
      error.value = saveError instanceof Error ? saveError.message : String(saveError);
      throw saveError;
    } finally {
      saving.value = false;
    }
  }

  async function evaluate() {
    evaluating.value = true;
    error.value = null;
    try {
      const result = await evaluateListing(listingId);
      listing.value = result.listing;
      grades.value = await getListingGrades(listingId);
    } catch (evaluateError) {
      error.value = evaluateError instanceof Error ? evaluateError.message : String(evaluateError);
      throw evaluateError;
    } finally {
      evaluating.value = false;
    }
  }

  return {
    listing: readonly(listing),
    grades: readonly(grades),
    loading: readonly(loading),
    saving: readonly(saving),
    evaluating: readonly(evaluating),
    error: readonly(error),
    fetchAll,
    saveField,
    evaluate,
  };
}
