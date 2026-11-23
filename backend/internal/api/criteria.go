package api

import (
	"net/http"
	"propertyfinder/internal/service"
)

const defaultCriterionWeight = 1

// CriterionHandler serves the criterion endpoints (spec 01-04 Layer 1.3).
type CriterionHandler struct {
	service *service.CriterionService
}

// List handles GET /api/profiles/{profileId}/criteria.
func (handler *CriterionHandler) List(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	criteria, err := handler.service.List(request.Context(), profileID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, criteria)
}

// Create handles POST /api/profiles/{profileId}/criteria.
func (handler *CriterionHandler) Create(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	var body struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		Weight       *int   `json:"weight,omitempty"`
		IsSubjective bool   `json:"is_subjective"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}

	weight := defaultCriterionWeight
	if body.Weight != nil {
		weight = *body.Weight
	}

	criterion, err := handler.service.Create(request.Context(), profileID, body.Name, body.Description, weight, body.IsSubjective)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, criterion)
}

// Update handles PATCH /api/criteria/{criterionId}.
func (handler *CriterionHandler) Update(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseUUID(writer, request, "criterionId")
	if !ok {
		return
	}

	var body struct {
		Name         *string `json:"name,omitempty"`
		Description  *string `json:"description,omitempty"`
		Weight       *int    `json:"weight,omitempty"`
		IsSubjective *bool   `json:"is_subjective,omitempty"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}

	criterion, err := handler.service.Update(request.Context(), id, body.Name, body.Description, body.Weight, body.IsSubjective)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, criterion)
}

// Delete handles DELETE /api/criteria/{criterionId}.
func (handler *CriterionHandler) Delete(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseUUID(writer, request, "criterionId")
	if !ok {
		return
	}

	if err := handler.service.Delete(request.Context(), id); err != nil {
		writeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

// Approve handles POST /api/criteria/{criterionId}/approve: turns a proposed
// criterion into an active one (docs/adr/0024).
func (handler *CriterionHandler) Approve(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseUUID(writer, request, "criterionId")
	if !ok {
		return
	}

	criterion, err := handler.service.Approve(request.Context(), id)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, criterion)
}
