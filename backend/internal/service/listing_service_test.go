package service

import (
	"errors"
	"propertyfinder/internal/domain"
	"slices"
	"testing"
	"time"
)

func newPatchTestListing() *domain.Listing {
	return &domain.Listing{
		Title:               "Haus",
		SourceURL:           new("https://example.test/1"),
		Price:               new(250000.0),
		Address:             new("Rua 1"),
		SellerPhone:         new("+351 123"),
		SellerNotes:         new("freundlich"),
		LastContactAt:       &time.Time{},
		Status:              domain.ListingStatusNew,
		UserConfirmedFields: []string{},
	}
}

// TestApplyPatch_ExplicitNullClearsField is the regression test for issue
// #9: an explicit null must clear an optional field (it used to be ignored
// because it was indistinguishable from an omitted key), while omitted
// fields stay untouched.
func TestApplyPatch_ExplicitNullClearsField(t *testing.T) {
	listing := newPatchTestListing()
	patch := ListingPatch{
		SellerPhone:   Clear[string](),
		Price:         Clear[float64](),
		LastContactAt: Clear[time.Time](),
	}

	if err := (&ListingService{}).applyPatch(listing, patch); err != nil {
		t.Fatalf("applyPatch: %v", err)
	}

	if listing.SellerPhone != nil {
		t.Errorf("SellerPhone: got %q, want nil", *listing.SellerPhone)
	}
	if listing.Price != nil {
		t.Errorf("Price: got %v, want nil", *listing.Price)
	}
	if listing.LastContactAt != nil {
		t.Errorf("LastContactAt: got %v, want nil", *listing.LastContactAt)
	}
	if listing.SellerNotes == nil || *listing.SellerNotes != "freundlich" {
		t.Errorf("SellerNotes (omitted): got %v, want unchanged", listing.SellerNotes)
	}
	if listing.Address == nil || *listing.Address != "Rua 1" {
		t.Errorf("Address (omitted): got %v, want unchanged", listing.Address)
	}
}

func TestApplyPatch_ClearTracksConfirmedFieldsAndReevaluation(t *testing.T) {
	listing := newPatchTestListing()

	if err := (&ListingService{}).applyPatch(listing, ListingPatch{Price: Clear[float64]()}); err != nil {
		t.Fatalf("applyPatch: %v", err)
	}
	if !slices.Contains(listing.UserConfirmedFields, "price") {
		t.Errorf("UserConfirmedFields: got %v, want it to contain price", listing.UserConfirmedFields)
	}
	if !listing.NeedsReevaluation {
		t.Error("clearing an evaluation-relevant field must set NeedsReevaluation")
	}

	listing = newPatchTestListing()
	if err := (&ListingService{}).applyPatch(listing, ListingPatch{SellerPhone: Clear[string]()}); err != nil {
		t.Fatalf("applyPatch: %v", err)
	}
	if listing.NeedsReevaluation {
		t.Error("clearing an administrative field must not set NeedsReevaluation")
	}
}

func TestApplyPatch_EmptyPatchChangesNothing(t *testing.T) {
	listing := newPatchTestListing()

	if err := (&ListingService{}).applyPatch(listing, ListingPatch{}); err != nil {
		t.Fatalf("applyPatch: %v", err)
	}
	if listing.SellerPhone == nil || listing.Price == nil || listing.SourceURL == nil {
		t.Error("an empty patch must not clear any field")
	}
	if len(listing.UserConfirmedFields) != 0 {
		t.Errorf("UserConfirmedFields: got %v, want empty", listing.UserConfirmedFields)
	}
}

func TestApplyPatch_SetValueStillWorks(t *testing.T) {
	listing := newPatchTestListing()

	if err := (&ListingService{}).applyPatch(listing, ListingPatch{Address: Some("Rua 2")}); err != nil {
		t.Fatalf("applyPatch: %v", err)
	}
	if listing.Address == nil || *listing.Address != "Rua 2" {
		t.Errorf("Address: got %v, want Rua 2", listing.Address)
	}
}

// TestApplyPatch_ClearingRejectionCommentOfRejectedListingFails keeps the
// "rejected requires a comment" invariant (spec 01-07) when the comment is
// cleared with null.
func TestApplyPatch_ClearingRejectionCommentOfRejectedListingFails(t *testing.T) {
	listing := newPatchTestListing()
	listing.Status = domain.ListingStatusRejected
	listing.RejectionComment = new("zu teuer")

	err := (&ListingService{}).applyPatch(listing, ListingPatch{RejectionComment: Clear[string]()})
	if err == nil {
		t.Fatal("expected a validation error")
	}
	if _, ok := errors.AsType[*domain.ValidationError](err); !ok {
		t.Errorf("got %T, want *domain.ValidationError", err)
	}
}
