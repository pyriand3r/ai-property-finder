import { http } from '@/api/http';

import type { ListingImage } from '@/types/listing';

export function listImages(listingId: string) {
  return http.get<ListingImage[]>(`/api/listings/${listingId}/images`);
}
