import { shallowRef, watch } from 'vue';

import { listImages } from '@/api/images';

import type { Listing } from '@/types/listing';

/** Resolves and caches title-image thumbnail URLs for a reactive set of
 * listings (List View, Rangliste). title_image_path (a carried-over column,
 * ADR 0012) is never populated by the actual image pipeline — only
 * title_image_id is, via download_image / the gallery's star icon — so
 * resolving a thumbnail means fetching each listing's images and matching
 * title_image_id, same as ChatListingCard.vue does for a single listing. */
export function useListingThumbnails(listings: () => readonly Listing[]) {
  const thumbnails = shallowRef<Map<string, string | null>>(new Map());

  watch(
    listings,
    (currentListings) => {
      const missing = currentListings.filter(
        (listing) => listing.title_image_id && !thumbnails.value.has(listing.id),
      );
      if (missing.length === 0) {
        return;
      }
      void Promise.all(
        missing.map(async (listing) => {
          try {
            const images = await listImages(listing.id);
            const titleImage = images.find((image) => image.id === listing.title_image_id);
            return [listing.id, titleImage ? `/media/${titleImage.file_path}` : null] as const;
          } catch {
            return [listing.id, null] as const;
          }
        }),
      ).then((resolved) => {
        const next = new Map(thumbnails.value);
        for (const [id, url] of resolved) {
          next.set(id, url);
        }
        thumbnails.value = next;
      });
    },
    { immediate: true },
  );

  return { thumbnails };
}
