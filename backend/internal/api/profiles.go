package api

import (
	"net/http"
	"propertyfinder/internal/service"
)

// ProfileHandler serves the search_profile endpoints (spec 01-08 Layer 1.1).
type ProfileHandler struct {
	service *service.ProfileService
}

// List handles GET /api/profiles.
func (handler *ProfileHandler) List(writer http.ResponseWriter, request *http.Request) {
	profiles, err := handler.service.List(request.Context())
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, profiles)
}

// Create handles POST /api/profiles.
func (handler *ProfileHandler) Create(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}

	profile, err := handler.service.Create(request.Context(), body.Name, body.Description)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, profile)
}

// Get handles GET /api/profiles/{profileId}.
func (handler *ProfileHandler) Get(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	profile, err := handler.service.Get(request.Context(), id)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, profile)
}

// Update handles PATCH /api/profiles/{profileId}.
func (handler *ProfileHandler) Update(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	var body struct {
		Name        *string `json:"name,omitempty"`
		Description *string `json:"description,omitempty"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}

	profile, err := handler.service.Update(request.Context(), id, body.Name, body.Description)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, profile)
}

// Archive handles PATCH /api/profiles/{profileId}/archive.
func (handler *ProfileHandler) Archive(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	profile, err := handler.service.Archive(request.Context(), id)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, profile)
}

// Delete handles DELETE /api/profiles/{profileId}.
func (handler *ProfileHandler) Delete(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	if err := handler.service.Delete(request.Context(), id); err != nil {
		writeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
