package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"propertyfinder/internal/db/dbtest"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// TestCriterionHandler_Approve_ActivatesProposal drives the REST path the
// Criteria tab uses: a proposal becomes active, a second approve is a 409,
// an unknown id is a 404.
func TestCriterionHandler_Approve_ActivatesProposal(t *testing.T) {
	pool := dbtest.NewPool(t)
	ctx := context.Background()
	profileID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO search_profile (id, name) VALUES ($1, 'P')`, profileID); err != nil {
		t.Fatal(err)
	}
	criteria := service.NewCriterionService(pool)
	proposed, err := criteria.Propose(ctx, profileID, "Lage", "", 2, true)
	if err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	router.Post("/api/criteria/{criterionId}/approve", (&CriterionHandler{service: criteria}).Approve)
	approve := func(id uuid.UUID) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/criteria/"+id.String()+"/approve", http.NoBody))
		return recorder
	}

	first := approve(proposed.ID)
	if first.Code != http.StatusOK {
		t.Fatalf("approve: status %d, body %s", first.Code, first.Body)
	}
	var body domain.Criterion
	if err := json.NewDecoder(first.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != domain.CriterionStatusActive || body.ID != proposed.ID {
		t.Errorf("approve response = %+v, want the same criterion, active", body)
	}
	if got := approve(proposed.ID).Code; got != http.StatusConflict {
		t.Errorf("second approve: status %d, want 409", got)
	}
	if got := approve(uuid.New()).Code; got != http.StatusNotFound {
		t.Errorf("approve of unknown id: status %d, want 404", got)
	}
}
