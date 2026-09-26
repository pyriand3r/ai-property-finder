import { http } from '@/api/http';

import type { ListingNote } from '@/types/listing';

export function listNotes(listingId: string) {
  return http.get<ListingNote[]>(`/api/listings/${listingId}/notes`);
}

export function createNote(listingId: string, content: string) {
  return http.post<ListingNote>(`/api/listings/${listingId}/notes`, { content });
}

export function updateNote(noteId: string, content: string) {
  return http.patch<ListingNote>(`/api/notes/${noteId}`, { content });
}

export function deleteNote(noteId: string) {
  return http.delete<void>(`/api/notes/${noteId}`);
}
