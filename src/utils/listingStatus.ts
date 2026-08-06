import type { ListingStatus } from '@/types/listing';

/** Shared status -> German label mapping. Was previously duplicated as a local
 * `statusOptions` array in ListingListTable.vue and ListingDetailPanel.vue —
 * centralized here once Map View markers needed the same labels, following
 * the same pattern as gradeColor.ts (centralize on 2nd/3rd use). */
export const statusLabel: Record<ListingStatus, string> = {
  new: 'Neu',
  waiting: 'Wartend',
  in_negotiation: 'In Verhandlung',
  visited: 'Besichtigt',
  rejected: 'Abgelehnt',
  archived: 'Archiviert',
};

export const statusOptions: { label: string; value: ListingStatus }[] = (
  Object.keys(statusLabel) as ListingStatus[]
).map((value) => ({ label: statusLabel[value], value }));

/** Status -> marker/badge color (Map View markers, spec 01-06). Distinct
 * hues per status so markers are distinguishable at a glance; unrelated to
 * gradeColor.ts's A-F scale (status isn't a quality gradient). */
export const statusColor: Record<ListingStatus, string> = {
  new: '#2080f0',
  waiting: '#f0a020',
  in_negotiation: '#9254de',
  visited: '#18a058',
  rejected: '#d03050',
  archived: '#808080',
};
