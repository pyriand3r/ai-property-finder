package api

import (
	"net/http"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"
)

// Defaults of a new saved search (spec 01-17): daily, first page only, up to
// three new links handed to the Agent per run.
const (
	defaultSavedSearchIntervalHours = 24
	defaultSavedSearchMaxPages      = 1
	defaultSavedSearchMaxImport     = 3
)

// SavedSearchHandler serves the saved portal search endpoints (spec 01-17).
type SavedSearchHandler struct {
	service *service.SavedSearchService
}

// List handles GET /api/profiles/{profileId}/saved-searches.
func (handler *SavedSearchHandler) List(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}
	searches, err := handler.service.List(request.Context(), profileID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, searches)
}

// Create handles POST /api/profiles/{profileId}/saved-searches. Omitted
// optional fields get their defaults; a search is enabled and imports new
// links unless the body says otherwise.
func (handler *SavedSearchHandler) Create(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}
	var body struct {
		Name          string `json:"name"`
		URL           string `json:"url"`
		PageParam     string `json:"page_param"`
		MaxPages      *int   `json:"max_pages,omitempty"`
		LinkContains  string `json:"link_contains"`
		IntervalHours *int   `json:"interval_hours,omitempty"`
		Enabled       *bool  `json:"enabled,omitempty"`
		ImportNew     *bool  `json:"import_new,omitempty"`
		MaxImport     *int   `json:"max_import,omitempty"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}

	input := domain.SavedSearchInput{
		Name: body.Name, URL: body.URL, PageParam: body.PageParam, LinkContains: body.LinkContains,
		MaxPages: defaultSavedSearchMaxPages, IntervalHours: defaultSavedSearchIntervalHours,
		Enabled: true, ImportNew: true, MaxImport: defaultSavedSearchMaxImport,
	}
	if body.MaxPages != nil {
		input.MaxPages = *body.MaxPages
	}
	if body.IntervalHours != nil {
		input.IntervalHours = *body.IntervalHours
	}
	if body.Enabled != nil {
		input.Enabled = *body.Enabled
	}
	if body.ImportNew != nil {
		input.ImportNew = *body.ImportNew
	}
	if body.MaxImport != nil {
		input.MaxImport = *body.MaxImport
	}

	search, err := handler.service.Create(request.Context(), profileID, input)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, search)
}

// Update handles PATCH /api/profiles/{profileId}/saved-searches/{searchId}.
func (handler *SavedSearchHandler) Update(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}
	searchID, ok := parseUUID(writer, request, "searchId")
	if !ok {
		return
	}
	var body struct {
		Name          *string `json:"name,omitempty"`
		URL           *string `json:"url,omitempty"`
		PageParam     *string `json:"page_param,omitempty"`
		MaxPages      *int    `json:"max_pages,omitempty"`
		LinkContains  *string `json:"link_contains,omitempty"`
		IntervalHours *int    `json:"interval_hours,omitempty"`
		Enabled       *bool   `json:"enabled,omitempty"`
		ImportNew     *bool   `json:"import_new,omitempty"`
		MaxImport     *int    `json:"max_import,omitempty"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}

	search, err := handler.service.Update(request.Context(), profileID, searchID, service.SavedSearchPatch{
		Name: body.Name, URL: body.URL, PageParam: body.PageParam, MaxPages: body.MaxPages,
		LinkContains: body.LinkContains, IntervalHours: body.IntervalHours, Enabled: body.Enabled,
		ImportNew: body.ImportNew, MaxImport: body.MaxImport,
	})
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, search)
}

// Delete handles DELETE /api/profiles/{profileId}/saved-searches/{searchId}.
func (handler *SavedSearchHandler) Delete(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}
	searchID, ok := parseUUID(writer, request, "searchId")
	if !ok {
		return
	}
	if err := handler.service.Delete(request.Context(), profileID, searchID); err != nil {
		writeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

// Run handles POST /api/profiles/{profileId}/saved-searches/{searchId}/run:
// starts a run now and returns 202 with the search, marked running. The
// outcome shows in the search once the run is done. 409 if it is running.
func (handler *SavedSearchHandler) Run(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}
	searchID, ok := parseUUID(writer, request, "searchId")
	if !ok {
		return
	}
	search, err := handler.service.RunNow(request.Context(), profileID, searchID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, search)
}
