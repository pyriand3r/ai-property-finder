package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"propertyfinder/internal/db/dbtest"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"
	"slices"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// TestHazardHandler_Lookups_ReturnRegionsAndRatings drives the two REST
// lookups the hazard UI (issue #45) will call, against the dataset that
// ships in the binary (docs/adr/0025).
func TestHazardHandler_Lookups_ReturnRegionsAndRatings(t *testing.T) {
	pool := dbtest.NewPool(t)
	hazards := service.NewHazardService(pool)
	if err := hazards.SeedStatic(context.Background()); err != nil {
		t.Fatalf("SeedStatic: %v", err)
	}
	handler := &HazardHandler{service: hazards}
	router := chi.NewRouter()
	router.Get("/api/hazards/regions", handler.SearchRegions)
	router.Get("/api/hazards/regions/{regionKey}", handler.GetRegion)
	get := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		return recorder
	}

	t.Run("region with ratings in hazard order, each with its source", func(t *testing.T) {
		recorder := get("/api/hazards/regions/PT-1106")
		if recorder.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", recorder.Code, recorder.Body)
		}
		var region domain.RegionHazards
		if err := json.Unmarshal(recorder.Body.Bytes(), &region); err != nil {
			t.Fatal(err)
		}
		if region.Key != "PT-1106" || region.Name != "Lisboa" || region.Country != "PT" {
			t.Errorf("region = %+v, want PT-1106 Lisboa", region.HazardRegion)
		}
		if len(region.Ratings) == 0 {
			t.Fatal("Lisboa has no ratings")
		}
		order := domain.HazardTypes()
		last := -1
		for _, rating := range region.Ratings {
			position := slices.Index(order, rating.HazardType)
			if position <= last {
				t.Errorf("ratings %+v are not in domain.HazardTypes order", region.Ratings)
			}
			last = position
			if rating.Rating < domain.HazardRatingMin || rating.Rating > domain.HazardRatingMax ||
				rating.Source.ID == "" || rating.Source.URL == "" || rating.Source.Method == "" || rating.UpdatedAt.IsZero() {
				t.Errorf("rating %+v is out of range or lacks its source, method or timestamp", rating)
			}
		}
	})

	t.Run("search key is never sent to clients", func(t *testing.T) {
		var regions []map[string]any
		if err := json.Unmarshal(get("/api/hazards/regions?country=PT&q=evora").Body.Bytes(), &regions); err != nil {
			t.Fatal(err)
		}
		if len(regions) != 1 || regions[0]["name"] != "Évora" || regions[0]["key"] != "PT-0705" {
			t.Fatalf("regions = %v, want Évora (PT-0705) found without the accent", regions)
		}
		if _, leaked := regions[0]["name_search"]; leaked {
			t.Errorf("response %v exposes the internal name_search column", regions[0])
		}
	})

	t.Run("a shared name returns every region with it", func(t *testing.T) {
		var regions []domain.HazardRegion
		if err := json.Unmarshal(get("/api/hazards/regions?country=PT&q=Lagoa").Body.Bytes(), &regions); err != nil {
			t.Fatal(err)
		}
		if len(regions) != 2 || regions[0].ParentName == regions[1].ParentName {
			t.Errorf("regions = %+v, want the two Lagoa municipalities with different parents", regions)
		}
	})

	t.Run("no match is an empty list, not null", func(t *testing.T) {
		recorder := get("/api/hazards/regions?q=no-such-place")
		if recorder.Code != http.StatusOK || recorder.Body.String() != "[]\n" {
			t.Errorf("status %d body %q, want 200 and []", recorder.Code, recorder.Body)
		}
	})

	t.Run("errors map to their status codes", func(t *testing.T) {
		for path, want := range map[string]int{
			"/api/hazards/regions/PT-9999":          http.StatusNotFound,
			"/api/hazards/regions/lisboa":           http.StatusBadRequest,
			"/api/hazards/regions?country=Portugal": http.StatusBadRequest,
		} {
			if got := get(path).Code; got != want {
				t.Errorf("GET %s: status %d, want %d", path, got, want)
			}
		}
	})
}

// TestHazardHandler_Sources_CatalogAndSuggestionReview drives the REST side of
// docs/adr/0027: the catalog, the suggestion list and the approve/reject
// decisions, which a second decision on the same suggestion must refuse.
func TestHazardHandler_Sources_CatalogAndSuggestionReview(t *testing.T) {
	pool := dbtest.NewPool(t)
	sources := service.NewHazardSourceService(pool)
	handler := &HazardHandler{service: service.NewHazardService(pool), sources: sources}
	router := chi.NewRouter()
	router.Get("/api/hazards/sources", handler.ListSources)
	router.Get("/api/hazards/source-suggestions", handler.ListSuggestions)
	router.Post("/api/hazards/source-suggestions/{suggestionId}/approve", handler.ApproveSuggestion)
	router.Post("/api/hazards/source-suggestions/{suggestionId}/reject", handler.RejectSuggestion)
	do := func(method, path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(method, path, http.NoBody))
		return recorder
	}

	var catalog []domain.HazardCatalogEntry
	if err := json.Unmarshal(do(http.MethodGet, "/api/hazards/sources").Body.Bytes(), &catalog); err != nil || len(catalog) == 0 {
		t.Fatalf("catalog = %v, %v, want entries", catalog, err)
	}

	suggestion, err := sources.Suggest(context.Background(), domain.HazardSourceSuggestionInput{
		Country: "SE", HazardType: domain.HazardEarthquake, Title: "SNSN", Publisher: "Uppsala universitet",
		URL: "https://snsn.se", Rationale: "Erdbebenkatalog",
	})
	if err != nil {
		t.Fatal(err)
	}
	var proposed []domain.HazardSourceSuggestion
	if err = json.Unmarshal(do(http.MethodGet, "/api/hazards/source-suggestions?status=proposed").Body.Bytes(), &proposed); err != nil || len(proposed) != 1 {
		t.Fatalf("proposed = %v, %v, want the one suggestion", proposed, err)
	}
	if got := do(http.MethodGet, "/api/hazards/source-suggestions?status=bogus").Code; got != http.StatusBadRequest {
		t.Errorf("unknown status filter: status %d, want 400", got)
	}

	approve := "/api/hazards/source-suggestions/" + suggestion.ID.String() + "/approve"
	if recorder := do(http.MethodPost, approve); recorder.Code != http.StatusOK {
		t.Fatalf("approve: status %d, body %s", recorder.Code, recorder.Body)
	}
	if got := do(http.MethodPost, "/api/hazards/source-suggestions/"+suggestion.ID.String()+"/reject").Code; got != http.StatusConflict {
		t.Errorf("second decision: status %d, want 409", got)
	}
	if got := do(http.MethodPost, "/api/hazards/source-suggestions/not-a-uuid/approve").Code; got != http.StatusBadRequest {
		t.Errorf("bad id: status %d, want 400", got)
	}
	if got := do(http.MethodPost, "/api/hazards/source-suggestions/"+uuid.New().String()+"/approve").Code; got != http.StatusNotFound {
		t.Errorf("unknown id: status %d, want 404", got)
	}
}
