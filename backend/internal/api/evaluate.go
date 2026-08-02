package api

import (
	"net/http"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"
)

// EvaluationHandler serves the single-shot evaluate_listing endpoint (spec
// 01-04 Layer 2.2) and bulk re-evaluation (Layer 2.3).
type EvaluationHandler struct {
	service *service.EvaluationService
}

type evaluateResponse struct {
	Listing *domain.Listing         `json:"listing"`
	Grades  []domain.CriterionScore `json:"grades"`
}

// Evaluate handles POST /api/listings/{listingId}/evaluate.
func (handler *EvaluationHandler) Evaluate(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseUUID(writer, request, "listingId")
	if !ok {
		return
	}

	result, err := handler.service.EvaluateListing(request.Context(), id)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, evaluateResponse{Listing: result.Listing, Grades: result.Grades})
}

// ReevaluateAll handles POST /api/profiles/{profileId}/listings/reevaluate-all.
// Returns 202 immediately with the created task; progress streams over
// GET /api/events (task.progress / task.finished).
func (handler *EvaluationHandler) ReevaluateAll(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	task, err := handler.service.ReevaluateAll(request.Context(), profileID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, task)
}
