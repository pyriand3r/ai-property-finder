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
		ContentMD string `json:"contentMd"`
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
		ContentMD string `json:"contentMd"`
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
