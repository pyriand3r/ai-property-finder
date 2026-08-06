package api

import (
	"net/http"
	"propertyfinder/internal/service"
)

// ContextHandler serves the Context File endpoints (spec 01-02 Layer 1.2).
type ContextHandler struct {
	service *service.ContextService
}

// GetGlobal handles GET /api/context/global.
func (handler *ContextHandler) GetGlobal(writer http.ResponseWriter, request *http.Request) {
	contextRow, err := handler.service.GetGlobal(request.Context())
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, contextRow)
}

// PutGlobal handles PUT /api/context/global.
func (handler *ContextHandler) PutGlobal(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		ContentMD string `json:"content_md"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}

	contextRow, err := handler.service.UpdateGlobal(request.Context(), body.ContentMD)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, contextRow)
}

// GetProfile handles GET /api/profiles/{profileId}/context.
func (handler *ContextHandler) GetProfile(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	contextRow, err := handler.service.GetProfileContext(request.Context(), profileID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, contextRow)
}

// PutProfile handles PUT /api/profiles/{profileId}/context.
func (handler *ContextHandler) PutProfile(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	var body struct {
		ContentMD string `json:"content_md"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}

	contextRow, err := handler.service.UpdateProfileContext(request.Context(), profileID, body.ContentMD)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, contextRow)
}

// History handles GET /api/profiles/{profileId}/context/history.
func (handler *ContextHandler) History(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	revisions, err := handler.service.ListProfileHistory(request.Context(), profileID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, revisions)
}

// GlobalHistory handles GET /api/context/global/history.
func (handler *ContextHandler) GlobalHistory(writer http.ResponseWriter, request *http.Request) {
	revisions, err := handler.service.ListGlobalHistory(request.Context())
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, revisions)
}

// RestoreGlobal handles POST /api/context/global/revisions/{revisionId}/restore.
func (handler *ContextHandler) RestoreGlobal(writer http.ResponseWriter, request *http.Request) {
	revisionID, ok := parseUUID(writer, request, "revisionId")
	if !ok {
		return
	}

	contextRow, err := handler.service.RestoreGlobal(request.Context(), revisionID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, contextRow)
}

// RestoreProfile handles
// POST /api/profiles/{profileId}/context/revisions/{revisionId}/restore.
func (handler *ContextHandler) RestoreProfile(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}
	revisionID, ok := parseUUID(writer, request, "revisionId")
	if !ok {
		return
	}

	contextRow, err := handler.service.RestoreProfileContext(request.Context(), profileID, revisionID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, contextRow)
}
