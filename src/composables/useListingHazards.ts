import { shallowRef, watch } from 'vue';

import { getRegionHazards, listHazardRegions } from '@/api/hazards';
import { resolveHazardRegion } from '@/utils/hazard';

import type { HazardRegion, RegionHazards } from '@/types/hazard';
import type { Listing } from '@/types/listing';

type HazardListing = Pick<Listing, 'id' | 'region' | 'country' | 'address'>;

/**
 * Resolves the Hazard Region of each listing and loads its ratings (spec 01-13, issue #45).
 * Ratings belong to the region, never to the listing, so a region shared by many listings is
 * fetched once. `hazards` holds an entry only for listings whose region was found; a listing
 * without a region or without data simply has none. Static data: loaded once per listing id.
 *
 * Failures (no hazard data, offline) leave the listing without ratings instead of raising an
 * error: the hazard columns are supplementary, the list must stay usable.
 */
export function useListingHazards(listings: () => readonly HazardListing[]) {
  const hazards = shallowRef<ReadonlyMap<string, RegionHazards>>(new Map());

  let regions: Promise<HazardRegion[]> | null = null;
  const regionRatings = new Map<string, Promise<RegionHazards | null>>();
  const attempted = new Map<string, string>();

  function loadRegionRatings(key: string): Promise<RegionHazards | null> {
    let pending = regionRatings.get(key);
    if (!pending) {
      pending = getRegionHazards(key).catch(() => null);
      regionRatings.set(key, pending);
    }
    return pending;
  }

  async function resolve(pending: readonly HazardListing[]) {
    regions ??= listHazardRegions().catch(() => []);
    const allRegions = await regions;
    const resolved = await Promise.all(
      pending.map(async (listing) => {
        const region = resolveHazardRegion(listing, allRegions);
        return [listing.id, region ? await loadRegionRatings(region.key) : null] as const;
      }),
    );
    const next = new Map(hazards.value);
    for (const [id, ratings] of resolved) {
      if (ratings) {
        next.set(id, ratings);
      } else {
        next.delete(id);
      }
    }
    hazards.value = next;
  }

  watch(
    listings,
    (current) => {
      // A listing is looked up again when the text its region is derived from changed.
      const pending = current.filter((listing) => {
        const signature = `${listing.region ?? ''}|${listing.country ?? ''}|${listing.address ?? ''}`;
        if (attempted.get(listing.id) === signature) {
          return false;
        }
        attempted.set(listing.id, signature);
        return true;
      });
      if (pending.length === 0) {
        return;
      }
      void resolve(pending);
    },
    { immediate: true },
  );

  return { hazards };
}
