package api

import (
	"net/http"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"
)

// RankingHandler serves the ranking-refresh endpoint (spec 01-05 Layer 5.2).
type RankingHandler struct {
	service *service.RankingService
}

type rankingResponse struct {
	Profile  *domain.SearchProfile `json:"profile"`
	Listings []domain.Listing      `json:"listings"`
}

// Refresh handles POST /api/profiles/{profileId}/ranking/refresh.
func (handler *RankingHandler) Refresh(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	result, err := handler.service.Refresh(request.Context(), profileID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, rankingResponse{Profile: result.Profile, Listings: result.Listings})
}
