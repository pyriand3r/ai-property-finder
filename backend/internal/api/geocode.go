package api

import (
	"errors"
	"net/http"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/geocode"
	"strings"
)

// GeocodeHandler serves the address-to-coordinates lookup (ADR 0011 Layer
// 3.4). Stateless — calls internal/geocode directly, no service layer
// needed for a single passthrough HTTP call.
type GeocodeHandler struct{}

type geocodeResponse struct {
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	DisplayName string  `json:"display_name"`
}

// Get handles GET /api/geocode?address=...&country=...
func (handler *GeocodeHandler) Get(writer http.ResponseWriter, request *http.Request) {
	address := strings.TrimSpace(request.URL.Query().Get("address"))
	if address == "" {
		writeError(writer, &domain.ValidationError{Field: "address", Message: "address ist erforderlich"})
		return
	}
	country := request.URL.Query().Get("country")

	result, err := geocode.Geocode(request.Context(), address, country)
	if err != nil {
		if errors.Is(err, geocode.ErrNotFound) {
			writeError(writer, &domain.ValidationError{Field: "address", Message: "Adresse nicht gefunden"})
			return
		}
		writeError(writer, &domain.UpstreamError{Message: "Geocoding-Dienst nicht erreichbar"})
		return
	}
	writeJSON(writer, http.StatusOK, geocodeResponse{Lat: result.Lat, Lon: result.Lon, DisplayName: result.DisplayName})
}
