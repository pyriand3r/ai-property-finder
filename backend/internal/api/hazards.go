package api

import (
	"net/http"
	"propertyfinder/internal/service"

	"github.com/go-chi/chi/v5"
)

// HazardHandler serves the region-level natural-hazard lookups (docs/adr/
// 0025, spec 01-13). Read-only: the data is static and synced at boot.
type HazardHandler struct {
	service *service.HazardService
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
