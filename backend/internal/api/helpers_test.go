package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"propertyfinder/internal/domain"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestWriteError_DuplicateListingIsConflictWithMatches covers issue #10: the
// frontend needs the matching listings in the 409 body, and a stable code to
// tell this warning apart from other conflicts.
func TestWriteError_DuplicateListingIsConflictWithMatches(t *testing.T) {
	id := uuid.New()
	recorder := httptest.NewRecorder()

	writeError(recorder, &domain.DuplicateListingError{Duplicates: []domain.DuplicateListing{
		{ID: id, Title: "Haus", Matches: []domain.DuplicateMatch{domain.DuplicateMatchAddress}},
	}})

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want %d", recorder.Code, http.StatusConflict)
	}
	var body duplicateWarningBody
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != "duplicate_listing" {
		t.Errorf("code: got %q, want duplicate_listing", body.Code)
	}
	if len(body.Duplicates) != 1 || body.Duplicates[0].ID != id {
		t.Errorf("duplicates: got %+v, want one entry with id %s", body.Duplicates, id)
	}
}

type errorBody struct {
	Error string `json:"error"`
}

func TestWriteError_MapsErrorsToStatusAndBody(t *testing.T) {
	tests := map[string]struct {
		err        error
		wantStatus int
		wantError  string
	}{
		"validation": {&domain.ValidationError{Field: "name", Message: "Name fehlt"}, http.StatusBadRequest, "Name fehlt"},
		"not found":  {&domain.NotFoundError{Entity: "listing", ID: "42"}, http.StatusNotFound, "listing not found: 42"},
		"conflict":   {&domain.ConflictError{Message: "Name vergeben"}, http.StatusConflict, "Name vergeben"},
		"upstream":   {&domain.UpstreamError{Message: "LLM down"}, http.StatusBadGateway, "LLM down"},
		"wrapped":    {fmt.Errorf("svc: %w", &domain.NotFoundError{Entity: "note", ID: "1"}), http.StatusNotFound, "note not found: 1"},
		"unique violation": {
			fmt.Errorf("profile create: %w", &pgconn.PgError{Code: pgerrcode.UniqueViolation}),
			http.StatusConflict, "Ein Eintrag mit diesem Wert existiert bereits",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			writeError(recorder, test.err)

			if recorder.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			var body errorBody
			if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Error != test.wantError {
				t.Errorf("error = %q, want %q", body.Error, test.wantError)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
		})
	}
}

// TestWriteError_UnmappedErrorDoesNotLeakInternals: AGENTS.md — a 500 body
// must be generic; the detail only goes to the server log.
func TestWriteError_UnmappedErrorDoesNotLeakInternals(t *testing.T) {
	recorder := httptest.NewRecorder()

	writeError(recorder, errors.New("pq: password authentication failed for user app"))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "password") || !strings.Contains(recorder.Body.String(), "Internal server error") {
		t.Errorf("body = %s, want the generic message only", recorder.Body)
	}
}

func TestWriteError_OtherPostgresErrorIs500(t *testing.T) {
	recorder := httptest.NewRecorder()

	writeError(recorder, &pgconn.PgError{Code: pgerrcode.ForeignKeyViolation})

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", recorder.Code)
	}
}

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	t.Run("valid", func(t *testing.T) {
		var dest payload
		recorder := httptest.NewRecorder()

		ok := decodeJSON(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x"}`)), &dest)

		if !ok || dest.Name != "x" || recorder.Body.Len() != 0 {
			t.Errorf("ok=%v dest=%+v body=%q", ok, dest, recorder.Body)
		}
	})
	for name, body := range map[string]string{"malformed": `{"name":`, "empty": ``, "wrong type": `{"name":5}`} {
		t.Run(name, func(t *testing.T) {
			var dest payload
			recorder := httptest.NewRecorder()

			ok := decodeJSON(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), &dest)

			if ok || recorder.Code != http.StatusBadRequest {
				t.Errorf("ok=%v status=%d, want false and 400", ok, recorder.Code)
			}
		})
	}
}

func requestWithURLParam(name, value string) *http.Request {
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add(name, value)
	request := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}

func TestParseUUID(t *testing.T) {
	id := uuid.New()
	t.Run("valid, surrounding whitespace trimmed", func(t *testing.T) {
		recorder := httptest.NewRecorder()

		got, ok := parseUUID(recorder, requestWithURLParam("id", " "+id.String()+" "), "id")

		if !ok || got != id {
			t.Errorf("got (%v, %v), want (%v, true)", got, ok, id)
		}
	})
	for name, raw := range map[string]string{"garbage": "not-a-uuid", "empty": ""} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			got, ok := parseUUID(recorder, requestWithURLParam("id", raw), "id")

			if ok || got != uuid.Nil || recorder.Code != http.StatusBadRequest {
				t.Errorf("got (%v, %v) status %d, want (Nil, false) and 400", got, ok, recorder.Code)
			}
		})
	}
}
