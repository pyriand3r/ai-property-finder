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

func duplicateTestListing(title string, address *string, latitude, longitude *float64) domain.Listing {
	return domain.Listing{Title: title, Address: address, Latitude: latitude, Longitude: longitude}
}

// TestFindDuplicates_MatchesAddressIgnoringCaseAndWhitespace covers issue
// #10: a manually typed address rarely matches byte for byte.
func TestFindDuplicates_MatchesAddressIgnoringCaseAndWhitespace(t *testing.T) {
	existing := []domain.Listing{
		duplicateTestListing("A", new("Rua Nova  1, Porto"), nil, nil),
		duplicateTestListing("B", new("Rua Velha 2, Porto"), nil, nil),
	}

	got := findDuplicates(existing, new(" rua nova 1,   PORTO "), nil, nil)

	if len(got) != 1 || got[0].Title != "A" {
		t.Fatalf("got %+v, want only listing A", got)
	}
	if !slices.Equal(got[0].Matches, []domain.DuplicateMatch{domain.DuplicateMatchAddress}) {
		t.Errorf("Matches: got %v, want [address]", got[0].Matches)
	}
}

func TestFindDuplicates_MatchesCoordinates(t *testing.T) {
	existing := []domain.Listing{
		duplicateTestListing("near", nil, new(41.1496), new(-8.6109)),
		duplicateTestListing("far", nil, new(41.1596), new(-8.6109)),
	}

	got := findDuplicates(existing, nil, new(41.1496), new(-8.6109))

	if len(got) != 1 || got[0].Title != "near" {
		t.Fatalf("got %+v, want only listing near", got)
	}
	if !slices.Equal(got[0].Matches, []domain.DuplicateMatch{domain.DuplicateMatchCoordinates}) {
		t.Errorf("Matches: got %v, want [coordinates]", got[0].Matches)
	}
}

func TestFindDuplicates_ReportsBothMatchKinds(t *testing.T) {
	existing := []domain.Listing{duplicateTestListing("A", new("Rua 1"), new(1.0), new(2.0))}

	got := findDuplicates(existing, new("Rua 1"), new(1.0), new(2.0))

	if len(got) != 1 || len(got[0].Matches) != 2 {
		t.Fatalf("got %+v, want one listing matching on address and coordinates", got)
	}
}

// TestFindDuplicates_IgnoresEmptyAndPartialInput makes sure missing data is
// not a match: two listings without an address are not duplicates, and a
// lone latitude is not a coordinate pair.
func TestFindDuplicates_IgnoresEmptyAndPartialInput(t *testing.T) {
	existing := []domain.Listing{
		duplicateTestListing("no address", new("  "), new(1.0), new(2.0)),
		duplicateTestListing("nil address", nil, nil, nil),
	}

	if got := findDuplicates(existing, new("   "), nil, nil); len(got) != 0 {
		t.Errorf("blank address: got %+v, want none", got)
	}
	if got := findDuplicates(existing, nil, nil, nil); len(got) != 0 {
		t.Errorf("no input: got %+v, want none", got)
	}
	if got := findDuplicates(existing, nil, new(1.0), nil); len(got) != 0 {
		t.Errorf("latitude only: got %+v, want none", got)
	}
}
