package service

import (
	"context"
	"errors"
	"propertyfinder/internal/db/dbtest"
	"propertyfinder/internal/domain"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func suggestionInput(url string) domain.HazardSourceSuggestionInput {
	return domain.HazardSourceSuggestionInput{
		Country: "se", HazardType: domain.HazardEarthquake, Scope: "Schweden", Title: "SNSN",
		Publisher: "Uppsala universitet", URL: url, Rationale: "Erdbebenkatalog",
	}
}

// TestHazardSourceService_Suggest_RejectsInvalidInput needs no database: the
// checks run before any query.
func TestHazardSourceService_Suggest_RejectsInvalidInput(t *testing.T) {
	service := NewHazardSourceService(nil)
	cases := map[string]func(*domain.HazardSourceSuggestionInput){
		"country":     func(in *domain.HazardSourceSuggestionInput) { in.Country = "SWE" },
		"hazard_type": func(in *domain.HazardSourceSuggestionInput) { in.HazardType = "tsunami" },
		"url":         func(in *domain.HazardSourceSuggestionInput) { in.URL = "ftp://x.test" },
		"title":       func(in *domain.HazardSourceSuggestionInput) { in.Title = " " },
		"publisher":   func(in *domain.HazardSourceSuggestionInput) { in.Publisher = "" },
		"rationale":   func(in *domain.HazardSourceSuggestionInput) { in.Rationale = strings.Repeat("x", 2001) },
	}
	for field, modify := range cases {
		input := suggestionInput("https://snsn.se")
		modify(&input)
		_, err := service.Suggest(context.Background(), input)
		var validation *domain.ValidationError
		if !errors.As(err, &validation) || validation.Field != field {
			t.Errorf("%s: err = %v, want a ValidationError for that field", field, err)
		}
	}
}

// TestHazardSourceService_SuggestAndReview covers the whole workflow of docs/
// adr/0027 against a real Postgres: duplicates are refused (catalog and
// earlier suggestions, whatever their status), a decision is final, and an
// unknown id is a 404.
func TestHazardSourceService_SuggestAndReview(t *testing.T) {
	pool := dbtest.NewPool(t)
	service := NewHazardSourceService(pool)
	ctx := context.Background()

	first, err := service.Suggest(ctx, suggestionInput("https://www.snsn.se/"))
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if first.Status != domain.HazardSuggestionProposed || first.Country != "SE" || first.DecidedAt != nil {
		t.Errorf("new suggestion = %+v, want proposed, country SE, undecided", first)
	}

	t.Run("same source again is a conflict, even in another spelling", func(t *testing.T) {
		_, suggestError := service.Suggest(ctx, suggestionInput("http://snsn.se"))
		var conflict *domain.ConflictError
		if !errors.As(suggestError, &conflict) || !strings.Contains(conflict.Message, "proposed") {
			t.Errorf("err = %v, want a ConflictError naming the status", suggestError)
		}
	})
	t.Run("same url for another hazard type is a different source", func(t *testing.T) {
		input := suggestionInput("https://snsn.se")
		input.HazardType = domain.HazardLandslide
		if _, suggestError := service.Suggest(ctx, input); suggestError != nil {
			t.Errorf("err = %v, want nil", suggestError)
		}
	})
	t.Run("a source of the catalog is a conflict", func(t *testing.T) {
		input := suggestionInput("https://hora.gv.at/")
		input.Country, input.HazardType = "AT", domain.HazardFlood
		_, suggestError := service.Suggest(ctx, input)
		var conflict *domain.ConflictError
		if !errors.As(suggestError, &conflict) || !strings.Contains(conflict.Message, "at-flood-hora") {
			t.Errorf("err = %v, want a ConflictError naming the catalog entry", suggestError)
		}
	})

	approved, err := service.Approve(ctx, first.ID)
	if err != nil || approved.Status != domain.HazardSuggestionApproved || approved.DecidedAt == nil {
		t.Fatalf("Approve = %+v, %v, want approved with decided_at", approved, err)
	}
	if _, err = service.Reject(ctx, first.ID); err == nil {
		t.Error("deciding twice must fail")
	}
	var notFound *domain.NotFoundError
	if _, err = service.Approve(ctx, uuid.New()); !errors.As(err, &notFound) {
		t.Errorf("unknown id: err = %v, want NotFoundError", err)
	}

	rejected, err := service.Suggest(ctx, suggestionInput("https://other.se"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Reject(ctx, rejected.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Suggest(ctx, suggestionInput("https://other.se")); err == nil {
		t.Error("a rejected source must not be suggestable again")
	}

	proposed, err := service.ListSuggestions(ctx, "proposed")
	if err != nil || len(proposed) != 1 || proposed[0].HazardType != domain.HazardLandslide {
		t.Errorf("proposed list = %+v, %v, want only the landslide suggestion", proposed, err)
	}
	all, err := service.ListSuggestions(ctx, "")
	if err != nil || len(all) != 3 {
		t.Errorf("full list has %d rows (%v), want 3", len(all), err)
	}
	var validation *domain.ValidationError
	if _, err = service.ListSuggestions(ctx, "bogus"); !errors.As(err, &validation) {
		t.Errorf("unknown status filter: err = %v, want ValidationError", err)
	}
}
