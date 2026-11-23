package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"propertyfinder/internal/domain"
	"testing"

	"github.com/google/uuid"
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
