import type { SavedSearchStatus } from '@/types/savedSearch';

export const savedSearchStatusLabel: Record<SavedSearchStatus, string> = {
  idle: 'Noch nicht gelaufen',
  running: 'Läuft',
  succeeded: 'Erfolgreich',
  failed: 'Fehlgeschlagen',
};

export const savedSearchStatusType: Record<
  SavedSearchStatus,
  'default' | 'info' | 'success' | 'error'
> = {
  idle: 'default',
  running: 'info',
  succeeded: 'success',
  failed: 'error',
};

/** German label of a schedule given in hours, e.g. 24 -> "täglich", 36 -> "alle 36 Stunden". */
export function intervalLabel(hours: number): string {
  if (hours === 1) return 'stündlich';
  if (hours === 24) return 'täglich';
  if (hours === 168) return 'wöchentlich';
  if (hours % 24 === 0) return `alle ${hours / 24} Tage`;
  return `alle ${hours} Stunden`;
}

/** Local date and time of an API timestamp, or an en dash when there is none. */
export function formatDateTime(value: string | undefined): string {
  if (!value) return '–';
  return new Date(value).toLocaleString('de-DE', { dateStyle: 'short', timeStyle: 'short' });
}
