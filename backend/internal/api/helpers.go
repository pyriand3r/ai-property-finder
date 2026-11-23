package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"propertyfinder/internal/domain"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// writeJSON serializes data as JSON and writes the given status.
func writeJSON(writer http.ResponseWriter, status int, data any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(data)
}

// apiError is the JSON body returned for any non-2xx response.
type apiError struct {
	Error string `json:"error"`
}

// duplicateWarningBody is the 409 body for a possible duplicate listing:
// the usual error text plus the matching listings (issue #10).
type duplicateWarningBody struct {
	Error      string                    `json:"error"`
	Code       string                    `json:"code"`
	Duplicates []domain.DuplicateListing `json:"duplicates"`
}

// writeError maps a domain/service error to an HTTP status code.
func writeError(writer http.ResponseWriter, err error) {
	var validationError *domain.ValidationError
	var notFoundError *domain.NotFoundError
	var conflictError *domain.ConflictError
	var upstreamError *domain.UpstreamError
	var duplicateError *domain.DuplicateListingError

	switch {
	case errors.As(err, &duplicateError):
		writeJSON(writer, http.StatusConflict, duplicateWarningBody{
			Error:      duplicateError.Error(),
			Code:       "duplicate_listing",
			Duplicates: duplicateError.Duplicates,
		})
	case errors.As(err, &validationError):
		writeJSON(writer, http.StatusBadRequest, apiError{Error: validationError.Message})
	case errors.As(err, &notFoundError):
		writeJSON(writer, http.StatusNotFound, apiError{Error: notFoundError.Error()})
	case errors.As(err, &conflictError):
		writeJSON(writer, http.StatusConflict, apiError{Error: conflictError.Message})
	case errors.As(err, &upstreamError):
		writeJSON(writer, http.StatusBadGateway, apiError{Error: upstreamError.Message})
	case isUniqueViolation(err):
		// A unique constraint fired despite the service's pre-check (a race
		// between two concurrent creates) — surface it as a 409, not a 500.
		writeJSON(writer, http.StatusConflict, apiError{Error: "Ein Eintrag mit diesem Wert existiert bereits"})
	default:
		log.Printf("internal server error: %v", err)
		writeJSON(writer, http.StatusInternalServerError, apiError{Error: "Internal server error"})
	}
}

// isUniqueViolation reports whether err wraps a Postgres unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation
}

// decodeJSON decodes the request body into dest, writing a 400 on failure.
func decodeJSON(writer http.ResponseWriter, request *http.Request, dest any) bool {
	if err := json.NewDecoder(request.Body).Decode(dest); err != nil {
		writeError(writer, &domain.ValidationError{Field: "body", Message: "Ungültiger Request-Body"})
		return false
	}
	return true
}

// parseUUID extracts and validates a UUID URL parameter, writing a 400 to
// the response and returning ok=false on failure.
func parseUUID(writer http.ResponseWriter, request *http.Request, name string) (uuid.UUID, bool) {
	raw := strings.TrimSpace(chi.URLParam(request, name))
	id, err := uuid.Parse(raw)
	if err != nil {
		writeError(writer, &domain.ValidationError{Field: name, Message: "invalid UUID: " + raw})
		return uuid.Nil, false
	}
	return id, true
}
