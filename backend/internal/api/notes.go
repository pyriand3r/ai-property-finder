package api

import (
	"net/http"
	"propertyfinder/internal/service"
)

// NoteHandler serves the listing note endpoints (spec 01-11, RF.4).
type NoteHandler struct {
	service *service.NoteService
}

// List handles GET /api/listings/{listingId}/notes.
func (handler *NoteHandler) List(writer http.ResponseWriter, request *http.Request) {
	listingID, ok := parseUUID(writer, request, "listingId")
	if !ok {
		return
	}

	notes, err := handler.service.List(request.Context(), listingID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, notes)
}

// Create handles POST /api/listings/{listingId}/notes.
func (handler *NoteHandler) Create(writer http.ResponseWriter, request *http.Request) {
	listingID, ok := parseUUID(writer, request, "listingId")
	if !ok {
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}

	note, err := handler.service.Create(request.Context(), listingID, body.Content)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, note)
}

// Update handles PATCH /api/notes/{noteId}.
func (handler *NoteHandler) Update(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseUUID(writer, request, "noteId")
	if !ok {
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}

	note, err := handler.service.Update(request.Context(), id, body.Content)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, note)
}

// Delete handles DELETE /api/notes/{noteId}.
func (handler *NoteHandler) Delete(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseUUID(writer, request, "noteId")
	if !ok {
		return
	}

	if err := handler.service.Delete(request.Context(), id); err != nil {
		writeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
