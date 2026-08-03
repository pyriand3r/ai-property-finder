package api

import (
	"net/http"
	"propertyfinder/internal/service"
)

// ChatHandler serves Chat Session/message persistence (spec 01-01) and
// triggers the Agent turn on each new message (spec 00-03, Layer 6.3).
type ChatHandler struct {
	service *service.ChatService
	agent   *service.AgentService
}

// ListSessions handles GET /api/profiles/{profileId}/chat/sessions.
func (handler *ChatHandler) ListSessions(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	sessions, err := handler.service.ListSessions(request.Context(), profileID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, sessions)
}

// StartSession handles POST /api/profiles/{profileId}/chat/sessions — ends
// the current active session and starts a new one.
func (handler *ChatHandler) StartSession(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	session, err := handler.service.StartNewSession(request.Context(), profileID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, session)
}

// ListMessages handles
// GET /api/profiles/{profileId}/chat/sessions/{sessionId}/messages.
func (handler *ChatHandler) ListMessages(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}
	sessionID, ok := parseUUID(writer, request, "sessionId")
	if !ok {
		return
	}

	messages, err := handler.service.ListMessages(request.Context(), profileID, sessionID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, messages)
}

// SendMessage handles
// POST /api/profiles/{profileId}/chat/sessions/{sessionId}/messages —
// persists the user message, then fires the Agent turn as a detached
// goroutine (ADR 0008). Returns 202 Accepted with the persisted user
// message the moment it's written; the reply streams over GET /api/events
// (chat.text_delta, chat.message, chat.turn_error) and lands as its own
// chat_message once the turn ends.
func (handler *ChatHandler) SendMessage(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}
	sessionID, ok := parseUUID(writer, request, "sessionId")
	if !ok {
		return
	}

	var body struct {
		Content        string  `json:"content"`
		AttachmentPath *string `json:"attachment_path,omitempty"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}

	message, err := handler.agent.SendMessage(request.Context(), profileID, sessionID, body.Content, body.AttachmentPath)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, message)
}
