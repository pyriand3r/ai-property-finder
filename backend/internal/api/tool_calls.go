package api

import (
	"net/http"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// ToolCallHandler serves the confirm/reject endpoints for pending
// propose_criteria/update_context proposals (ADR 0009/0004, Layer 6.5).
type ToolCallHandler struct {
	service *service.ToolCallService
}

// Confirm handles
// POST /api/chat/messages/{messageId}/tool-calls/{callIdx}/confirm.
func (handler *ToolCallHandler) Confirm(writer http.ResponseWriter, request *http.Request) {
	messageID, ok := parseUUID(writer, request, "messageId")
	if !ok {
		return
	}
	callIndex, ok := parseCallIndex(writer, request)
	if !ok {
		return
	}

	result, err := handler.service.Confirm(request.Context(), messageID, callIndex)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

// Reject handles
// POST /api/chat/messages/{messageId}/tool-calls/{callIdx}/reject.
func (handler *ToolCallHandler) Reject(writer http.ResponseWriter, request *http.Request) {
	messageID, ok := parseUUID(writer, request, "messageId")
	if !ok {
		return
	}
	callIndex, ok := parseCallIndex(writer, request)
	if !ok {
		return
	}

	if err := handler.service.Reject(request.Context(), messageID, callIndex); err != nil {
		writeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func parseCallIndex(writer http.ResponseWriter, request *http.Request) (int, bool) {
	raw := chi.URLParam(request, "callIdx")
	index, err := strconv.Atoi(raw)
	if err != nil || index < 0 {
		writeError(writer, &domain.ValidationError{Field: "callIdx", Message: "ungültiger Tool-Call-Index: " + raw})
		return 0, false
	}
	return index, true
}
