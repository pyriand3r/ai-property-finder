import { readonly, shallowRef } from 'vue';

import { listListings } from '@/api/listings';

import type { Listing } from '@/types/listing';

/** Loads a profile's full listing set (List View's data source — every
 * listing regardless of status, spec 01-05). */
export function useListings(profileId: string) {
  const listings = shallowRef<Listing[]>([]);
  const loading = shallowRef(false);
  const error = shallowRef<string | null>(null);

  async function fetchListings() {
    loading.value = true;
    error.value = null;
    try {
      listings.value = await listListings(profileId);
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    } finally {
      loading.value = false;
    }
  }

  /** Merges freshly updated listings (e.g. from a ranking refresh) back
   * into the local list by id, leaving untouched listings as-is. */
  function mergeListings(updated: readonly Listing[]) {
    const byId = new Map(updated.map((listing) => [listing.id, listing]));
    listings.value = listings.value.map((listing) => byId.get(listing.id) ?? listing);
  }

  return {
    listings: readonly(listings),
    loading: readonly(loading),
    error: readonly(error),
    fetchListings,
    mergeListings,
  };
}
