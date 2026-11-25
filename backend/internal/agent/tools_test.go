package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/fetch"
	"propertyfinder/internal/geocode"
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

func (f *fakeListingWriter) ListByProfile(_ context.Context, profileID uuid.UUID, _ *bool) ([]domain.Listing, error) {
	var result []domain.Listing
	for _, listing := range f.listings {
		if listing.ProfileID == profileID {
			result = append(result, *listing)
		}
	}
	return result, nil
}

func (f *fakeListingWriter) ListRankable(_ context.Context, profileID uuid.UUID) ([]domain.Listing, error) {
	var result []domain.Listing
	for _, listing := range f.listings {
		if listing.ProfileID == profileID && listing.OverallScore != nil {
			result = append(result, *listing)
		}
	}
	return result, nil
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

func dispatchText(registry *Registry, call ToolCall) (string, bool) {
	result := registry.Dispatch(context.Background(), call)
	return result.Result, result.IsError
}

func TestDispatch_FetchURL_ReturnsPageContent(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	registry.Fetcher = &fakePageFetcher{result: &fetch.Result{
		Title: "Kappmuren 2", Text: "Schönes Haus", ImageURLs: []string{"https://x/1.jpg"}, Links: []string{"https://x/more"},
	}}

	result, isError := dispatchText(registry, ToolCall{Name: "fetch_url", Arguments: json.RawMessage(`{"url":"https://x"}`)})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}
	if !strings.Contains(result, "Kappmuren 2") || !strings.Contains(result, "Schönes Haus") {
		t.Errorf("result = %s, want it to contain title and text", result)
	}
}

func TestDispatch_FetchURL_ReturnsLinkedPDFAsNativeDocument(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	pdfData := []byte("%PDF-1.4\n% floor-plan fixture")
	registry.Fetcher = &fakePageFetcher{result: &fetch.Result{PDFData: pdfData}}

	result := registry.Dispatch(context.Background(), ToolCall{Name: "fetch_url", Arguments: json.RawMessage(`{"url":"https://x/floorplan.pdf"}`)})
	if result.IsError {
		t.Fatalf("Dispatch returned an error result: %s", result.Result)
	}
	if want := base64.StdEncoding.EncodeToString(pdfData); result.DocumentData != want {
		t.Errorf("DocumentData = %q, want %q", result.DocumentData, want)
	}
	var payload struct {
		URL       string `json:"url"`
		MediaType string `json:"media_type"`
	}
	if err := json.Unmarshal([]byte(result.Result), &payload); err != nil {
		t.Fatalf("decode tool result: %v", err)
	}
	if payload.URL != "https://x/floorplan.pdf" || payload.MediaType != "application/pdf" {
		t.Errorf("tool result = %+v, want PDF URL and media type", payload)
	}
	if strings.Contains(result.Result, result.DocumentData) {
		t.Error("JSON tool result contains the base64 PDF; it must stay turn-local")
	}
}

func TestDispatch_FetchURL_PropagatesFetchError(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	registry.Fetcher = &fakePageFetcher{err: errors.New("blocked")}

	_, isError := dispatchText(registry, ToolCall{Name: "fetch_url", Arguments: json.RawMessage(`{"url":"https://x"}`)})
	if !isError {
		t.Error("expected an error result when the fetch fails")
	}
}

func TestDispatch_SaveListing_CreatesNewListing(t *testing.T) {
	listings := newFakeListingWriter()
	registry := newTestRegistry(t, listings, &fakeImageWriter{})

	args := `{"fields":{"title":"Kappmuren 2","price":310000,"price_currency":"SEK"}}`
	result, isError := dispatchText(registry, ToolCall{Name: "save_listing", Arguments: json.RawMessage(args)})
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
	result, isError := dispatchText(registry, ToolCall{Name: "save_listing", Arguments: json.RawMessage(args)})
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
	result, isError = dispatchText(registry, ToolCall{Name: "save_listing", Arguments: json.RawMessage(forcedArgs)})
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
	_, isError := dispatchText(registry, ToolCall{Name: "save_listing", Arguments: json.RawMessage(args)})
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
	result, isError := dispatchText(registry, ToolCall{Name: "download_image", Arguments: json.RawMessage(args)})
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
	_, isError := dispatchText(registry, ToolCall{Name: "download_image", Arguments: json.RawMessage(args)})
	if !isError {
		t.Error("expected an error result for an unrecognized category")
	}
}

func TestDispatch_UnknownTool_ReturnsError(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	_, isError := dispatchText(registry, ToolCall{Name: "does_not_exist"})
	if !isError {
		t.Error("expected an error result for an unknown tool name")
	}
}

type fakeEvaluator struct {
	listing *domain.Listing
	scores  []domain.CriterionScore
	err     error
}

func (f *fakeEvaluator) EvaluateListing(context.Context, uuid.UUID) (*domain.Listing, []domain.CriterionScore, error) {
	return f.listing, f.scores, f.err
}

func TestDispatch_EvaluateListing_ReturnsGrades(t *testing.T) {
	listings := newFakeListingWriter()
	registry := newTestRegistry(t, listings, &fakeImageWriter{})
	listingID := uuid.New()
	listings.listings[listingID] = &domain.Listing{ID: listingID, ProfileID: registry.ProfileID}

	score := 85.0
	registry.Evaluator = &fakeEvaluator{
		listing: &domain.Listing{ID: listingID, ProfileID: registry.ProfileID, OverallScore: &score},
		scores:  []domain.CriterionScore{{CriterionID: uuid.New(), Grade: domain.GradeA, Reasoning: "Gute Lage"}},
	}

	result, isError := dispatchText(registry, ToolCall{Name: "evaluate_listing", Arguments: json.RawMessage(`{"listing_id":"` + listingID.String() + `"}`)})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}
	if !strings.Contains(result, "Gute Lage") || !strings.Contains(result, `"grade":"A"`) {
		t.Errorf("result = %s, want it to contain the grade and reasoning", result)
	}
	if registry.TouchedListingID() == nil || *registry.TouchedListingID() != listingID {
		t.Error("TouchedListingID not set to the evaluated listing")
	}
}

func TestDispatch_EvaluateListing_RejectsListingFromAnotherProfile(t *testing.T) {
	listings := newFakeListingWriter()
	registry := newTestRegistry(t, listings, &fakeImageWriter{})
	registry.Evaluator = &fakeEvaluator{}

	listingID := uuid.New()
	listings.listings[listingID] = &domain.Listing{ID: listingID, ProfileID: uuid.New()}

	_, isError := dispatchText(registry, ToolCall{Name: "evaluate_listing", Arguments: json.RawMessage(`{"listing_id":"` + listingID.String() + `"}`)})
	if !isError {
		t.Error("expected an error result for a listing from another profile")
	}
}

// fakeCriterionProposer records Propose calls; err makes it fail like the
// service does on a duplicate name.
type fakeCriterionProposer struct {
	proposed []domain.Criterion
	err      error
}

func (f *fakeCriterionProposer) Propose(_ context.Context, profileID uuid.UUID, name, description string, weight int, isSubjective bool) (*domain.Criterion, error) {
	if f.err != nil {
		return nil, f.err
	}
	criterion := domain.Criterion{
		ID: uuid.New(), ProfileID: profileID, Name: name, Description: description,
		Weight: weight, IsSubjective: isSubjective, Status: domain.CriterionStatusProposed,
	}
	f.proposed = append(f.proposed, criterion)
	return &criterion, nil
}

// TestDispatch_ProposeCriteria_AddStoresProposedCriterion: action=add must
// write a *proposed* criterion at tool time (docs/adr/0024) and hand its id
// back; evaluation ignores it until the user approves it.
func TestDispatch_ProposeCriteria_AddStoresProposedCriterion(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	proposer := &fakeCriterionProposer{}
	registry.Criteria = proposer

	args := `{"action":"add","name":"Grundstück &amp; Lage","description":"Distanz zum See","is_subjective":false,"weight":3}`
	result, isError := dispatchText(registry, ToolCall{Name: "propose_criteria", Arguments: json.RawMessage(args)})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}
	if !strings.Contains(result, `"status":"pending_confirmation"`) || !strings.Contains(result, `"criterion_id"`) {
		t.Errorf("result = %s, want status=pending_confirmation and a criterion_id", result)
	}
	if len(proposer.proposed) != 1 {
		t.Fatalf("got %d proposed criteria, want 1", len(proposer.proposed))
	}
	got := proposer.proposed[0]
	if got.Name != "Grundstück & Lage" || got.Weight != 3 || got.ProfileID != registry.ProfileID {
		t.Errorf("proposed = %+v, want HTML-decoded name, weight 3, the registry's profile", got)
	}
}

func TestDispatch_ProposeCriteria_AddPropagatesDuplicateNameError(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	registry.Criteria = &fakeCriterionProposer{err: &domain.ConflictError{Message: "Kriterium existiert bereits: Preis"}}

	result, isError := dispatchText(registry, ToolCall{Name: "propose_criteria", Arguments: json.RawMessage(`{"action":"add","name":"Preis"}`)})
	if !isError || !strings.Contains(result, "existiert bereits") {
		t.Errorf("result = %s (isError=%v), want an error naming the duplicate", result, isError)
	}
}

// TestDispatch_ProposeCriteria_EditWritesNothing: an edit stays a pending
// Confirm-Card proposal (ADR 0009); only the confirm call changes the row.
func TestDispatch_ProposeCriteria_EditWritesNothing(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	proposer := &fakeCriterionProposer{}
	registry.Criteria = proposer

	args := `{"action":"edit","criterion_id":"` + uuid.NewString() + `","name":"Preis"}`
	result, isError := dispatchText(registry, ToolCall{Name: "propose_criteria", Arguments: json.RawMessage(args)})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}
	if !strings.Contains(result, `"status":"pending_confirmation"`) {
		t.Errorf("result = %s, want status=pending_confirmation", result)
	}
	if len(proposer.proposed) != 0 {
		t.Errorf("edit proposed %d criteria, want none", len(proposer.proposed))
	}
}

func TestDispatch_ProposeCriteria_RejectsEditWithoutCriterionID(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})

	args := `{"action":"edit","name":"Preis"}`
	_, isError := dispatchText(registry, ToolCall{Name: "propose_criteria", Arguments: json.RawMessage(args)})
	if !isError {
		t.Error("expected an error result for action=edit without criterion_id")
	}
}

func TestDispatch_ProposeCriteria_RejectsOutOfRangeWeight(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})

	args := `{"action":"add","name":"Preis","weight":9}`
	_, isError := dispatchText(registry, ToolCall{Name: "propose_criteria", Arguments: json.RawMessage(args)})
	if !isError {
		t.Error("expected an error result for weight out of 1-5 range")
	}
}

func TestDispatch_UpdateContext_ReturnsPendingConfirmation(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})

	args := `{"scope":"profile","operation":"append","content":"Bei Booli immer dem Makler-Link folgen."}`
	result, isError := dispatchText(registry, ToolCall{Name: "update_context", Arguments: json.RawMessage(args)})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}
	if !strings.Contains(result, `"status":"pending_confirmation"`) {
		t.Errorf("result = %s, want status=pending_confirmation", result)
	}
}

func TestDispatch_UpdateContext_RejectsUnsupportedOperation(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})

	args := `{"scope":"global","operation":"replace","content":"x"}`
	_, isError := dispatchText(registry, ToolCall{Name: "update_context", Arguments: json.RawMessage(args)})
	if !isError {
		t.Error("expected an error result for an unsupported operation")
	}
}

type fakeGeocoder struct {
	result *geocode.Result
	err    error
}

func (f *fakeGeocoder) Geocode(context.Context, string, string) (*geocode.Result, error) {
	return f.result, f.err
}

func TestDispatch_Geocode_ReturnsCoordinates(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	registry.Geocoder = &fakeGeocoder{result: &geocode.Result{Lat: 55.6, Lon: 13.0, DisplayName: "Malmö, Schweden"}}

	result, isError := dispatchText(registry, ToolCall{Name: "geocode", Arguments: json.RawMessage(`{"address":"Malmö"}`)})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}
	if !strings.Contains(result, "Malmö, Schweden") || !strings.Contains(result, "55.6") {
		t.Errorf("result = %s, want it to contain the resolved coordinates and display name", result)
	}
}

func TestDispatch_Geocode_NotFoundIsAnError(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	registry.Geocoder = &fakeGeocoder{err: geocode.ErrNotFound}

	result, isError := dispatchText(registry, ToolCall{Name: "geocode", Arguments: json.RawMessage(`{"address":"Nirgendwo"}`)})
	if !isError {
		t.Error("expected an error result when the address isn't found")
	}
	if !strings.Contains(result, "nicht gefunden") {
		t.Errorf("result = %s, want a not-found message", result)
	}
}

func TestDispatch_ListProfileListings_ReturnsOnlyCurrentProfile(t *testing.T) {
	listings := newFakeListingWriter()
	registry := newTestRegistry(t, listings, &fakeImageWriter{})

	ownID := uuid.New()
	listings.listings[ownID] = &domain.Listing{ID: ownID, ProfileID: registry.ProfileID, Title: "Kappmuren 2", SourceURL: new("https://x")}
	otherID := uuid.New()
	listings.listings[otherID] = &domain.Listing{ID: otherID, ProfileID: uuid.New(), Title: "Not yours"}

	result, isError := dispatchText(registry, ToolCall{Name: "list_profile_listings"})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}
	if !strings.Contains(result, "Kappmuren 2") {
		t.Errorf("result = %s, want it to include the profile's own listing", result)
	}
	if strings.Contains(result, "Not yours") {
		t.Errorf("result = %s, want it to exclude another profile's listing", result)
	}
}

func TestDispatch_ComputeRanking_OrdersByScoreDescending(t *testing.T) {
	listings := newFakeListingWriter()
	registry := newTestRegistry(t, listings, &fakeImageWriter{})

	low, high := 40.0, 90.0
	lowID, highID := uuid.New(), uuid.New()
	listings.listings[lowID] = &domain.Listing{ID: lowID, ProfileID: registry.ProfileID, Title: "Niedrig", OverallScore: &low}
	listings.listings[highID] = &domain.Listing{ID: highID, ProfileID: registry.ProfileID, Title: "Hoch", OverallScore: &high}

	result, isError := dispatchText(registry, ToolCall{Name: "compute_ranking"})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}
	if !strings.Contains(result, "Hoch") || !strings.Contains(result, "Niedrig") {
		t.Errorf("result = %s, want both listings present", result)
	}
}

type fakeHazardSuggester struct {
	got domain.HazardSourceSuggestionInput
	err error
}

func (f *fakeHazardSuggester) Suggest(_ context.Context, input domain.HazardSourceSuggestionInput) (*domain.HazardSourceSuggestion, error) {
	f.got = input
	if f.err != nil {
		return nil, f.err
	}
	return &domain.HazardSourceSuggestion{
		ID: uuid.New(), Country: input.Country, HazardType: input.HazardType, Title: input.Title,
		Status: domain.HazardSuggestionProposed,
	}, nil
}

// TestDispatch_SuggestHazardSource_StoresAProposalAndSaysItIsNotActive: the
// Tool only proposes (docs/adr/0027), so the result must tell the model the
// source is not ingested yet, and arguments must reach the service decoded
// (the model sometimes writes `&amp;`, docs/buglist.md issue #4).
func TestDispatch_SuggestHazardSource_StoresAProposalAndSaysItIsNotActive(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	suggester := &fakeHazardSuggester{}
	registry.HazardSources = suggester

	args := `{"country":"at","hazard_type":"Flood","title":"HORA","publisher":"Land &amp; Forst","url":"https://hora.gv.at","rationale":"HQ100"}`
	result, isError := dispatchText(registry, ToolCall{Name: "suggest_hazard_source", Arguments: json.RawMessage(args)})
	if isError {
		t.Fatalf("Dispatch returned an error result: %s", result)
	}
	if !strings.Contains(result, "pending_confirmation") || !strings.Contains(result, "Noch nicht eingebunden") {
		t.Errorf("result = %s, want pending_confirmation and a not-ingested notice", result)
	}
	if suggester.got.HazardType != domain.HazardFlood || suggester.got.Publisher != "Land & Forst" {
		t.Errorf("service got %+v, want hazard type flood and the unescaped publisher", suggester.got)
	}
}

func TestDispatch_SuggestHazardSource_ServiceErrorReachesTheModel(t *testing.T) {
	registry := newTestRegistry(t, newFakeListingWriter(), &fakeImageWriter{})
	registry.HazardSources = &fakeHazardSuggester{err: &domain.ConflictError{Message: "Die Quelle steht schon im Katalog (at-flood-hora, Status candidate)."}}

	result, isError := dispatchText(registry, ToolCall{Name: "suggest_hazard_source", Arguments: json.RawMessage(`{"country":"AT"}`)})
	if !isError || !strings.Contains(result, "schon im Katalog") {
		t.Errorf("result = %s (isError %v), want the service's message as an error", result, isError)
	}
}
