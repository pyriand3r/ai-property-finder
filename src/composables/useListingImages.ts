import { readonly, shallowRef } from 'vue';

import { deleteImage, listImages } from '@/api/images';
import { updateListing } from '@/api/listings';

import type { ListingImage } from '@/types/listing';

/** Loads a Listing's image gallery and exposes delete / "set as title
 * image" actions (ADR 0012, Layer 7.3). Setting the title image writes
 * through updateListing directly — title_image_id is explicitly excluded
 * from user_confirmed_fields/needs_reevaluation server-side (a display
 * choice, not a data correction), so this doesn't need to go through
 * ListingForm's batched-edit machinery. */
export function useListingImages(listingId: string) {
  const images = shallowRef<ListingImage[]>([]);
  const loading = shallowRef(false);
  const error = shallowRef<string | null>(null);

  async function fetchImages() {
    loading.value = true;
    error.value = null;
    try {
      images.value = await listImages(listingId);
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    } finally {
      loading.value = false;
    }
  }

  async function removeImage(imageId: string) {
    error.value = null;
    try {
      await deleteImage(listingId, imageId);
      images.value = images.value.filter((image) => image.id !== imageId);
    } catch (deleteError) {
      error.value = deleteError instanceof Error ? deleteError.message : String(deleteError);
      throw deleteError;
    }
  }

  async function setTitleImage(imageId: string) {
    error.value = null;
    try {
      await updateListing(listingId, { title_image_id: imageId });
    } catch (updateError) {
      error.value = updateError instanceof Error ? updateError.message : String(updateError);
      throw updateError;
    }
  }

  return {
    images: readonly(images),
    loading: readonly(loading),
    error: readonly(error),
    fetchImages,
    removeImage,
    setTitleImage,
  };
}
