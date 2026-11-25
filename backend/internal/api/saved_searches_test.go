package api

import (
	"bytes"
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

// TestSavedSearchHandler_CreateUpdateDelete drives the REST path of the
// Suchen tab: defaults for omitted fields, validation as 400, a duplicate
// name as 409, and another profile's search as 404.
func TestSavedSearchHandler_CreateUpdateDelete(t *testing.T) {
	pool := dbtest.NewPool(t)
	ctx := context.Background()
	profileID, otherID := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{profileID, otherID} {
		if _, err := pool.Exec(ctx, `INSERT INTO search_profile (id, name) VALUES ($1, $2)`, id, id.String()); err != nil {
			t.Fatal(err)
		}
	}
	handler := &SavedSearchHandler{service: service.NewSavedSearchService(pool, nil, nil)}
	router := chi.NewRouter()
	router.Route("/api/profiles/{profileId}/saved-searches", func(r chi.Router) {
		r.Get("/", handler.List)
		r.Post("/", handler.Create)
		r.Patch("/{searchId}", handler.Update)
		r.Delete("/{searchId}", handler.Delete)
	})
	call := func(method, path, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		return recorder
	}
	base := "/api/profiles/" + profileID.String() + "/saved-searches"

	created := call(http.MethodPost, base+"/", `{"name":"Algarve","url":"https://x.test/s"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", created.Code, created.Body)
	}
	var search domain.SavedSearch
	if err := json.NewDecoder(created.Body).Decode(&search); err != nil {
		t.Fatal(err)
	}
	if search.IntervalHours != 24 || search.MaxPages != 1 || search.MaxImport != 3 || !search.Enabled || !search.ImportNew || search.NextRunAt == nil {
		t.Errorf("created search = %+v, want the defaults (daily, 1 page, 3 imports, enabled, import on, scheduled)", search)
	}

	if got := call(http.MethodPost, base+"/", `{"name":"Algarve","url":"https://x.test/other"}`).Code; got != http.StatusConflict {
		t.Errorf("duplicate name: status %d, want 409", got)
	}
	if got := call(http.MethodPost, base+"/", `{"name":"B","url":"not a url"}`).Code; got != http.StatusBadRequest {
		t.Errorf("invalid url: status %d, want 400", got)
	}

	one := base + "/" + search.ID.String()
	patched := call(http.MethodPatch, one, `{"interval_hours":6,"enabled":false}`)
	var after domain.SavedSearch
	if err := json.NewDecoder(patched.Body).Decode(&after); err != nil || patched.Code != http.StatusOK {
		t.Fatalf("patch: status %d, err %v", patched.Code, err)
	}
	if after.IntervalHours != 6 || after.Enabled || after.NextRunAt != nil || after.Name != "Algarve" {
		t.Errorf("patched search = %+v, want interval 6, disabled, unscheduled, name kept", after)
	}

	foreign := "/api/profiles/" + otherID.String() + "/saved-searches/" + search.ID.String()
	if got := call(http.MethodDelete, foreign, "").Code; got != http.StatusNotFound {
		t.Errorf("delete through another profile: status %d, want 404", got)
	}
	if got := call(http.MethodDelete, one, "").Code; got != http.StatusNoContent {
		t.Errorf("delete: status %d, want 204", got)
	}
	if got := call(http.MethodPatch, one, `{}`).Code; got != http.StatusNotFound {
		t.Errorf("patch after delete: status %d, want 404", got)
	}
}
