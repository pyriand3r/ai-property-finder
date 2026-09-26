import { readonly, shallowRef } from 'vue';

import { createNote, deleteNote, listNotes, updateNote } from '@/api/notes';

import type { ListingNote } from '@/types/listing';

/** Loads a Listing's notes and exposes create/update/delete (spec 01-11,
 * RF.4). Notes are newest-first and each is independently saved — unlike
 * ListingForm's batched field edits, a note edit is its own PATCH call. */
export function useListingNotes(listingId: string) {
  const notes = shallowRef<ListingNote[]>([]);
  const loading = shallowRef(false);
  const error = shallowRef<string | null>(null);

  async function fetchNotes() {
    loading.value = true;
    error.value = null;
    try {
      notes.value = await listNotes(listingId);
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    } finally {
      loading.value = false;
    }
  }

  async function addNote(content: string) {
    error.value = null;
    try {
      const note = await createNote(listingId, content);
      notes.value = [note, ...notes.value];
      return note;
    } catch (createError) {
      error.value = createError instanceof Error ? createError.message : String(createError);
      throw createError;
    }
  }

  async function editNote(noteId: string, content: string) {
    error.value = null;
    try {
      const updated = await updateNote(noteId, content);
      notes.value = notes.value.map((note) => (note.id === noteId ? updated : note));
      return updated;
    } catch (updateError) {
      error.value = updateError instanceof Error ? updateError.message : String(updateError);
      throw updateError;
    }
  }

  async function removeNote(noteId: string) {
    error.value = null;
    try {
      await deleteNote(noteId);
      notes.value = notes.value.filter((note) => note.id !== noteId);
    } catch (deleteError) {
      error.value = deleteError instanceof Error ? deleteError.message : String(deleteError);
      throw deleteError;
    }
  }

  return {
    notes: readonly(notes),
    loading: readonly(loading),
    error: readonly(error),
    fetchNotes,
    addNote,
    editNote,
    removeNote,
  };
}
