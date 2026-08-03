package agent

import (
	"context"
	"encoding/json"
	"errors"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/fetch"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// fakeListingWriter is an in-memory stand-in for repository.ListingRepository
// (narrowed to ListingWriter), keyed by ID.
type fakeListingWriter struct {
	listings map[uuid.UUID]*domain.Listing
	titleSet map[uuid.UUID]uuid.UUID
}

func newFakeListingWriter() *fakeListingWriter {
	return &fakeListingWriter{listings: map[uuid.UUID]*domain.Listing{}, titleSet: map[uuid.UUID]uuid.UUID{}}
}

func (f *fakeListingWriter) Create(_ context.Context, listing *domain.Listing) error {
	stored := *listing
	f.listings[listing.ID] = &stored
	return nil
}

func (f *fakeListingWriter) GetByID(_ context.Context, id uuid.UUID) (*domain.Listing, error) {
	listing, ok := f.listings[id]
	if !ok {
		return nil, &domain.NotFoundError{Entity: "listing", ID: id.String()}
	}
	copied := *listing
	return &copied, nil
}

func (f *fakeListingWriter) Update(_ context.Context, listing *domain.Listing) error {
	if _, ok := f.listings[listing.ID]; !ok {
		return &domain.NotFoundError{Entity: "listing", ID: listing.ID.String()}
	}
	stored := *listing
	f.listings[listing.ID] = &stored
	return nil
}

func (f *fakeListingWriter) SetTitleImageIfUnset(_ context.Context, listingID, imageID uuid.UUID) (bool, error) {
	if _, ok := f.titleSet[listingID]; ok {
		return false, nil
	}
	f.titleSet[listingID] = imageID
	return true, nil
}

// fakeImageWriter is an in-memory stand-in for repository.ImageRepository
// (narrowed to ImageWriter).
type fakeImageWriter struct {
	images []domain.ListingImage
}

func (f *fakeImageWriter) Create(_ context.Context, image *domain.ListingImage) error {
	f.images = append(f.images, *image)
	return nil
}

func (f *fakeImageWriter) CountByListing(_ context.Context, listingID uuid.UUID) (int, error) {
	count := 0
	for _, image := range f.images {
		if image.ListingID == listingID {
			count++
		}
	}
	return count, nil
}

type fakePageFetcher struct {
	result *fetch.Result
	err    error
}

func (f *fakePageFetcher) FetchPage(context.Context, string) (*fetch.Result, error) {
	return f.result, f.err
}

type fakeImageDownloader struct {
	image *fetch.DownloadedImage
	err   error
}

func (f *fakeImageDownloader) DownloadImage(context.Context, string) (*fetch.DownloadedImage, error) {
	return f.image, f.err
}

func newTestRegistry(t *testing.T, listings *fakeListingWriter, images *fakeImageWriter) *Registry {
	t.Helper()
	return &Registry{
		ProfileID:  uuid.New(),
		Listings:   listings,
		Images:     images,
		Fetcher:    &fakePageFetcher{},
		Downloader: &fakeImageDownloader{},
		DataDir:    t.TempDir(),
	}
}

func TestDispatch_FetchURL_ReturnsPageContent(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	registry.Fetcher = &fakePageFetcher{result: &fetch.Result{
		Title: "Kappmuren 2", Text: "Schönes Haus", ImageURLs: []string{"https://x/1.jpg"}, Links: []string{"https://x/more"},
	}}

	result, isError := registry.Dispatch(context.Background(), ToolCall{Name: "fetch_url", Arguments: json.RawMessage(`{"url":"https://x"}`)})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}
	if !strings.Contains(result, "Kappmuren 2") || !strings.Contains(result, "Schönes Haus") {
		t.Errorf("result = %s, want it to contain title and text", result)
	}
}

func TestDispatch_FetchURL_PropagatesFetchError(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	registry.Fetcher = &fakePageFetcher{err: errors.New("blocked")}

	_, isError := registry.Dispatch(context.Background(), ToolCall{Name: "fetch_url", Arguments: json.RawMessage(`{"url":"https://x"}`)})
	if !isError {
		t.Error("expected an error result when the fetch fails")
	}
}

func TestDispatch_SaveListing_CreatesNewListing(t *testing.T) {
	listings := newFakeListingWriter()
	registry := newTestRegistry(t, listings, &fakeImageWriter{})

	args := `{"fields":{"title":"Kappmuren 2","price":310000,"price_currency":"SEK"}}`
	result, isError := registry.Dispatch(context.Background(), ToolCall{Name: "save_listing", Arguments: json.RawMessage(args)})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}

	if len(listings.listings) != 1 {
		t.Fatalf("got %d listings, want 1", len(listings.listings))
	}
	var listing *domain.Listing
	for _, l := range listings.listings {
		listing = l
	}
	if listing.Title != "Kappmuren 2" || listing.Price == nil || *listing.Price != 310000 {
		t.Errorf("listing = %+v, want Title=Kappmuren 2 Price=310000", listing)
	}
	if listing.SourceType != domain.SourceTypeURL {
		t.Errorf("SourceType = %q, want %q", listing.SourceType, domain.SourceTypeURL)
	}
	if registry.TouchedListingID() == nil || *registry.TouchedListingID() != listing.ID {
		t.Error("TouchedListingID not set to the created listing")
	}
}

func TestDispatch_SaveListing_SkipsUserConfirmedFieldUnlessForced(t *testing.T) {
	listings := newFakeListingWriter()
	registry := newTestRegistry(t, listings, &fakeImageWriter{})
	profileID := registry.ProfileID

	existingID := uuid.New()
	price := 310000.0
	listings.listings[existingID] = &domain.Listing{
		ID: existingID, ProfileID: profileID, Title: "Kappmuren 2",
		Price: &price, UserConfirmedFields: []string{"price"},
	}

	// Normal call: price is protected, should be skipped.
	args := `{"listing_id":"` + existingID.String() + `","fields":{"price":305000}}`
	result, isError := registry.Dispatch(context.Background(), ToolCall{Name: "save_listing", Arguments: json.RawMessage(args)})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}
	if !strings.Contains(result, `"skipped_fields":["price"]`) {
		t.Errorf("result = %s, want skipped_fields to list price", result)
	}
	if got := *listings.listings[existingID].Price; got != 310000 {
		t.Errorf("price = %v, want unchanged 310000 (protected)", got)
	}

	// Forced call: price should now be overwritten and unprotected.
	forcedArgs := `{"listing_id":"` + existingID.String() + `","fields":{"price":305000},"force_overwrite":["price"]}`
	result, isError = registry.Dispatch(context.Background(), ToolCall{Name: "save_listing", Arguments: json.RawMessage(forcedArgs)})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}
	updated := listings.listings[existingID]
	if updated.Price == nil || *updated.Price != 305000 {
		t.Errorf("price = %v, want 305000 after force_overwrite", updated.Price)
	}
	if len(updated.UserConfirmedFields) != 0 {
		t.Errorf("UserConfirmedFields = %v, want price removed after force_overwrite", updated.UserConfirmedFields)
	}
}

func TestDispatch_SaveListing_RejectsListingFromAnotherProfile(t *testing.T) {
	listings := newFakeListingWriter()
	registry := newTestRegistry(t, listings, &fakeImageWriter{})

	otherProfileListingID := uuid.New()
	listings.listings[otherProfileListingID] = &domain.Listing{ID: otherProfileListingID, ProfileID: uuid.New(), Title: "Not yours"}

	args := `{"listing_id":"` + otherProfileListingID.String() + `","fields":{"price":1}}`
	_, isError := registry.Dispatch(context.Background(), ToolCall{Name: "save_listing", Arguments: json.RawMessage(args)})
	if !isError {
		t.Error("expected an error result when updating a listing from another profile")
	}
}

func TestDispatch_DownloadImage_StoresFileAndRow(t *testing.T) {
	listings := newFakeListingWriter()
	images := &fakeImageWriter{}
	registry := newTestRegistry(t, listings, images)

	listingID := uuid.New()
	listings.listings[listingID] = &domain.Listing{ID: listingID, ProfileID: registry.ProfileID, Title: "Kappmuren 2"}
	registry.Downloader = &fakeImageDownloader{image: &fetch.DownloadedImage{Data: []byte("fake-jpeg-bytes"), Extension: ".jpg"}}

	args := `{"listing_id":"` + listingID.String() + `","image_url":"https://x/1.jpg","category":"exterior"}`
	result, isError := registry.Dispatch(context.Background(), ToolCall{Name: "download_image", Arguments: json.RawMessage(args)})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}

	if len(images.images) != 1 {
		t.Fatalf("got %d images, want 1", len(images.images))
	}
	image := images.images[0]
	if image.ListingID != listingID || image.Category == nil || *image.Category != "exterior" {
		t.Errorf("image = %+v, want ListingID=%v Category=exterior", image, listingID)
	}
	if registry.TouchedListingID() == nil || *registry.TouchedListingID() != listingID {
		t.Error("TouchedListingID not set to the listing the image was downloaded for")
	}
}

func TestDispatch_DownloadImage_RejectsUnknownCategory(t *testing.T) {
	listings := newFakeListingWriter()
	registry := newTestRegistry(t, listings, &fakeImageWriter{})
	listingID := uuid.New()
	listings.listings[listingID] = &domain.Listing{ID: listingID, ProfileID: registry.ProfileID}

	args := `{"listing_id":"` + listingID.String() + `","image_url":"https://x/1.jpg","category":"basement"}`
	_, isError := registry.Dispatch(context.Background(), ToolCall{Name: "download_image", Arguments: json.RawMessage(args)})
	if !isError {
		t.Error("expected an error result for an unrecognized category")
	}
}

func TestDispatch_UnknownTool_ReturnsError(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	_, isError := registry.Dispatch(context.Background(), ToolCall{Name: "does_not_exist"})
	if !isError {
		t.Error("expected an error result for an unknown tool name")
	}
}
