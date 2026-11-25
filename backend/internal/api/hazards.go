package api

import (
	"context"
	"net/http"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// HazardHandler serves the region-level natural-hazard lookups (docs/adr/
// 0025, spec 01-13). Read-only: the data is static and synced at boot.
type HazardHandler struct {
	service *service.HazardService
	sources *service.HazardSourceService
}

// SearchRegions handles GET /api/hazards/regions?country=PT&q=lagoa. Both
// query parameters are optional filters; the response is a plain list of
// regions (key, country, name, parent_name), ordered by name, so a caller can
// find the key of a place it only knows by name.
func (handler *HazardHandler) SearchRegions(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	regions, err := handler.service.SearchRegions(request.Context(), query.Get("country"), query.Get("q"))
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, regions)
}

// GetRegion handles GET /api/hazards/regions/{regionKey}: the region and its
// ratings, each with its source.
func (handler *HazardHandler) GetRegion(writer http.ResponseWriter, request *http.Request) {
	region, err := handler.service.GetRegionHazards(request.Context(), chi.URLParam(request, "regionKey"))
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, region)
}

// ListSources handles GET /api/hazards/sources: the source catalog, bundled
// and candidate entries of every country.
func (handler *HazardHandler) ListSources(writer http.ResponseWriter, _ *http.Request) {
	entries, err := handler.sources.Catalog()
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, entries)
}

// ListSuggestions handles GET /api/hazards/source-suggestions?status=: source
// suggestions by the Agent, newest first.
func (handler *HazardHandler) ListSuggestions(writer http.ResponseWriter, request *http.Request) {
	suggestions, err := handler.sources.ListSuggestions(request.Context(), request.URL.Query().Get("status"))
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, suggestions)
}

// ApproveSuggestion handles POST /api/hazards/source-suggestions/{id}/approve.
func (handler *HazardHandler) ApproveSuggestion(writer http.ResponseWriter, request *http.Request) {
	handler.decide(writer, request, handler.sources.Approve)
}

// RejectSuggestion handles POST /api/hazards/source-suggestions/{id}/reject.
func (handler *HazardHandler) RejectSuggestion(writer http.ResponseWriter, request *http.Request) {
	handler.decide(writer, request, handler.sources.Reject)
}

func (handler *HazardHandler) decide(
	writer http.ResponseWriter,
	request *http.Request,
	decide func(context.Context, uuid.UUID) (*domain.HazardSourceSuggestion, error),
) {
	id, ok := parseUUID(writer, request, "suggestionId")
	if !ok {
		return
	}
	suggestion, err := decide(request.Context(), id)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, suggestion)
}
