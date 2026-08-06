import { shallowRef, watch } from 'vue';

import { getListingGrades } from '@/api/listings';

import type { ListingGrade } from '@/types/grade';
import type { Listing } from '@/types/listing';

/** Resolves and caches per-criterion grades for a reactive set of listings
 * (List View's optional score columns, spec 01-10). There is no bulk grades
 * endpoint — only GET /api/listings/{id}/grades — so this is an N+1 fetch,
 * gated by `enabled` so it only runs once the user actually turns a
 * criterion column on. */
export function useListingGrades(listings: () => readonly Listing[], enabled: () => boolean) {
  const grades = shallowRef<Map<string, ListingGrade[]>>(new Map());

  watch(
    [listings, enabled],
    ([currentListings, isEnabled]) => {
      if (!isEnabled) {
        return;
      }
      const missing = currentListings.filter((listing) => !grades.value.has(listing.id));
      if (missing.length === 0) {
        return;
      }
      void Promise.all(
        missing.map(async (listing) => {
          try {
            return [listing.id, await getListingGrades(listing.id)] as [string, ListingGrade[]];
          } catch {
            return [listing.id, []] as [string, ListingGrade[]];
          }
        }),
      ).then((resolved) => {
        const next = new Map(grades.value);
        for (const [id, listingGrades] of resolved) {
          next.set(id, listingGrades);
        }
        grades.value = next;
      });
    },
    { immediate: true },
  );

  return { grades };
}
