package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"propertyfinder/internal/db"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// evaluationRelevantFields are the structured columns that feed Criteria
// evaluation (spec 01-07 "Editing any structured field that participates in
// Criteria evaluation sets needs_reevaluation = true"). Administrative
// fields (status, seller contact, last_contact_at, title_image_path) and
// document_md (prose, explicitly excluded by spec) are tracked in
// user_confirmed_fields on edit but never trigger re-evaluation.
var evaluationRelevantFields = map[string]bool{
	"title":            true,
	"price":            true,
	"price_currency":   true,
	"property_size_m2": true,
	"plot_size_m2":     true,
	"address":          true,
	"region":           true,
	"country":          true,
	"latitude":         true,
	"longitude":        true,
	"description":      true,
}

var validListingStatuses = map[domain.ListingStatus]bool{
	domain.ListingStatusNew:           true,
	domain.ListingStatusWaiting:       true,
	domain.ListingStatusInNegotiation: true,
	domain.ListingStatusVisited:       true,
	domain.ListingStatusRejected:      true,
	domain.ListingStatusArchived:      true,
}

// ListingPatch carries the manual-edit-form fields for PATCH
// /api/listings/{id} — every field the user can correct directly (spec
// 01-07), plus TitleImageID (ADR 0012's manual "star icon" path — Layer
// 3.2). overall_score, user_confirmed_fields, needs_reevaluation,
// source_type, source_file_path, and kurzfazit are system/Agent-managed
// and not exposed here.
//
// Nullable fields are Optional: absent leaves the field unchanged, an
// explicit null (Clear) sets it to NULL. Title, Status and TitleImageID are
// plain pointers — they can be changed but not cleared.
type ListingPatch struct {
	Title            *string
	SourceURL        Optional[string]
	Price            Optional[float64]
	PriceCurrency    Optional[string]
	PropertySizeM2   Optional[float64]
	PlotSizeM2       Optional[float64]
	Address          Optional[string]
	Region           Optional[string]
	Country          Optional[string]
	Latitude         Optional[float64]
	Longitude        Optional[float64]
	Description      Optional[string]
	TitleImagePath   Optional[string]
	TitleImageID     *uuid.UUID
	Status           *domain.ListingStatus
	RejectionComment Optional[string]
	SellerName       Optional[string]
	SellerPhone      Optional[string]
	SellerEmail      Optional[string]
	SellerNotes      Optional[string]
	LastContactAt    Optional[time.Time]
	DocumentMD       Optional[string]
}

// ListingService orchestrates Listing CRUD (spec 01-07 Layer 2.1).
type ListingService struct {
	pool              *pgxpool.Pool
	dataDir           string
	rejectionAnalysis *RejectionAnalysisService
}

// NewListingService constructs a ListingService wired to pool. dataDir is
// the root of the image storage volume (ADR 0012), used to remove a
// listing's image directory on delete. rejectionAnalysis is fired
// fire-and-forget (Layer 8.1) whenever Update transitions a listing's
// status to rejected with a comment attached.
func NewListingService(pool *pgxpool.Pool, dataDir string, rejectionAnalysis *RejectionAnalysisService) *ListingService {
	return &ListingService{pool: pool, dataDir: dataDir, rejectionAnalysis: rejectionAnalysis}
}

// List returns a profile's listings, most recently created first,
// optionally filtered by needs_reevaluation.
func (service *ListingService) List(ctx context.Context, profileID uuid.UUID, needsReevaluation *bool) ([]domain.Listing, error) {
	return repository.NewListingRepo(service.pool).ListByProfile(ctx, profileID, needsReevaluation)
}

// Get returns a single listing.
func (service *ListingService) Get(ctx context.Context, id uuid.UUID) (*domain.Listing, error) {
	return repository.NewListingRepo(service.pool).GetByID(ctx, id)
}

// Create validates and inserts a new listing. Status defaults to "new",
// source_type to "manual" — this endpoint is the plain-form fallback (ADR
// 0001), not chat ingestion.
func (service *ListingService) Create(ctx context.Context, profileID uuid.UUID, title string, patch ListingPatch) (*domain.Listing, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, &domain.ValidationError{Field: "title", Message: "Titel ist erforderlich"}
	}
	if validationError := validateNonNegative("price", patch.Price.Value); validationError != nil {
		return nil, validationError
	}
	if validationError := validateNonNegative("property_size_m2", patch.PropertySizeM2.Value); validationError != nil {
		return nil, validationError
	}
	if validationError := validateNonNegative("plot_size_m2", patch.PlotSizeM2.Value); validationError != nil {
		return nil, validationError
	}
	status := domain.ListingStatusNew
	if patch.Status != nil {
		if validationError := validateListingStatus(*patch.Status, patch.RejectionComment.Value); validationError != nil {
			return nil, validationError
		}
		status = *patch.Status
	}

	now := time.Now().UTC()
	listing := &domain.Listing{
		ID:                  uuid.New(),
		ProfileID:           profileID,
		Title:               title,
		SourceURL:           patch.SourceURL.Value,
		Price:               patch.Price.Value,
		PriceCurrency:       patch.PriceCurrency.Value,
		PropertySizeM2:      patch.PropertySizeM2.Value,
		PlotSizeM2:          patch.PlotSizeM2.Value,
		Address:             patch.Address.Value,
		Region:              patch.Region.Value,
		Country:             patch.Country.Value,
		Latitude:            patch.Latitude.Value,
		Longitude:           patch.Longitude.Value,
		Description:         patch.Description.Value,
		TitleImagePath:      patch.TitleImagePath.Value,
		Status:              status,
		RejectionComment:    patch.RejectionComment.Value,
		SellerName:          patch.SellerName.Value,
		SellerPhone:         patch.SellerPhone.Value,
		SellerEmail:         patch.SellerEmail.Value,
		SellerNotes:         patch.SellerNotes.Value,
		LastContactAt:       patch.LastContactAt.Value,
		DocumentMD:          patch.DocumentMD.Value,
		SourceType:          domain.SourceTypeManual,
		UserConfirmedFields: []string{},
		NeedsReevaluation:   false,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := repository.NewListingRepo(service.pool).Create(ctx, listing); err != nil {
		return nil, err
	}
	return listing, nil
}

// Update applies a partial edit. Every field present in patch (including one
// explicitly cleared with null) is appended
// to user_confirmed_fields (deduplicated) — a PATCH call is, by definition,
// the user confirming that field's value (spec 01-07, ADR 0016). Fields in
// evaluationRelevantFields additionally set needs_reevaluation = true;
// document_md is excluded from both (prose, not structured).
//
// TitleImageID is handled separately from the rest of the patch: it is
// never added to user_confirmed_fields and never triggers re-evaluation
// (ADR 0012 — this is a pure display choice the user makes with one
// click, not a correction the Agent needs to be warned about).
func (service *ListingService) Update(ctx context.Context, id uuid.UUID, patch ListingPatch) (*domain.Listing, error) {
	repo := repository.NewListingRepo(service.pool)
	listing, err := repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	wasRejected := listing.Status == domain.ListingStatusRejected

	if patch.TitleImageID != nil {
		image, imageError := repository.NewImageRepo(service.pool).GetByID(ctx, *patch.TitleImageID)
		if imageError != nil {
			return nil, imageError
		}
		if image.ListingID != id {
			return nil, &domain.ValidationError{Field: "title_image_id", Message: "Bild gehört nicht zu diesem Inserat"}
		}
	}

	if validationError := service.applyPatch(listing, patch); validationError != nil {
		return nil, validationError
	}
	listing.UpdatedAt = time.Now().UTC()

	txError := db.WithTx(ctx, service.pool, func(tx pgx.Tx) error {
		if updateError := repository.NewListingRepo(tx).Update(ctx, listing); updateError != nil {
			return updateError
		}
		if patch.TitleImageID != nil {
			return repository.NewListingRepo(tx).SetTitleImage(ctx, id, *patch.TitleImageID)
		}
		return nil
	})
	if txError != nil {
		return nil, txError
	}
	if patch.TitleImageID != nil {
		listing.TitleImageID = patch.TitleImageID
	}

	// Fire Layer 8.1's rejection analysis on the transition INTO rejected
	// only — not on every subsequent save of an already-rejected listing
	// (e.g. editing the seller phone afterward shouldn't re-fire it).
	nowRejected := listing.Status == domain.ListingStatusRejected
	if !wasRejected && nowRejected && listing.RejectionComment != nil && strings.TrimSpace(*listing.RejectionComment) != "" {
		listingID := listing.ID
		// Detached from the request context on purpose (ADR 0008
		// fire-and-forget shape): the goroutine must keep running after
		// this PATCH response is sent, not get canceled when the request
		// ends — same reasoning as EvaluationService.ReevaluateAll.
		//nolint:gosec,contextcheck // G118/non-inherited context: intentional (ADR 0008), not an oversight — see comment above.
		go func() {
			if analyzeError := service.rejectionAnalysis.AnalyzeRejection(context.Background(), listingID); analyzeError != nil {
				slog.Error("rejection analysis failed", "listing_id", listingID, "error", analyzeError)
			}
		}()
	}

	return listing, nil
}

// applyPatch merges patch onto listing in place, validating along the way
// and tracking user_confirmed_fields / needs_reevaluation.
func (service *ListingService) applyPatch(listing *domain.Listing, patch ListingPatch) error {
	if validationError := validateNonNegative("price", patch.Price.Value); validationError != nil {
		return validationError
	}
	if validationError := validateNonNegative("property_size_m2", patch.PropertySizeM2.Value); validationError != nil {
		return validationError
	}
	if validationError := validateNonNegative("plot_size_m2", patch.PlotSizeM2.Value); validationError != nil {
		return validationError
	}

	touch := func(field string) {
		listing.UserConfirmedFields = uniqueAppend(listing.UserConfirmedFields, field)
		if evaluationRelevantFields[field] {
			listing.NeedsReevaluation = true
		}
	}

	if patch.Title != nil {
		trimmed := strings.TrimSpace(*patch.Title)
		if trimmed == "" {
			return &domain.ValidationError{Field: "title", Message: "Titel darf nicht leer sein"}
		}
		listing.Title = trimmed
		touch("title")
	}
	if patch.SourceURL.Set {
		listing.SourceURL = patch.SourceURL.Value
		touch("source_url")
	}
	if patch.Price.Set {
		listing.Price = patch.Price.Value
		touch("price")
	}
	if patch.PriceCurrency.Set {
		listing.PriceCurrency = patch.PriceCurrency.Value
		touch("price_currency")
	}
	if patch.PropertySizeM2.Set {
		listing.PropertySizeM2 = patch.PropertySizeM2.Value
		touch("property_size_m2")
	}
	if patch.PlotSizeM2.Set {
		listing.PlotSizeM2 = patch.PlotSizeM2.Value
		touch("plot_size_m2")
	}
	if patch.Address.Set {
		listing.Address = patch.Address.Value
		touch("address")
	}
	if patch.Region.Set {
		listing.Region = patch.Region.Value
		touch("region")
	}
	if patch.Country.Set {
		listing.Country = patch.Country.Value
		touch("country")
	}
	if patch.Latitude.Set {
		listing.Latitude = patch.Latitude.Value
		touch("latitude")
	}
	if patch.Longitude.Set {
		listing.Longitude = patch.Longitude.Value
		touch("longitude")
	}
	if patch.Description.Set {
		listing.Description = patch.Description.Value
		touch("description")
	}
	if patch.TitleImagePath.Set {
		listing.TitleImagePath = patch.TitleImagePath.Value
		touch("title_image_path")
	}
	if patch.SellerName.Set {
		listing.SellerName = patch.SellerName.Value
		touch("seller_name")
	}
	if patch.SellerPhone.Set {
		listing.SellerPhone = patch.SellerPhone.Value
		touch("seller_phone")
	}
	if patch.SellerEmail.Set {
		listing.SellerEmail = patch.SellerEmail.Value
		touch("seller_email")
	}
	if patch.SellerNotes.Set {
		listing.SellerNotes = patch.SellerNotes.Value
		touch("seller_notes")
	}
	if patch.LastContactAt.Set {
		listing.LastContactAt = patch.LastContactAt.Value
		touch("last_contact_at")
	}
	if patch.DocumentMD.Set {
		// Prose, not a structured field (spec 01-07) — deliberately not
		// touch()'d: no user_confirmed_fields entry, no re-evaluation.
		listing.DocumentMD = patch.DocumentMD.Value
	}

	// Status/rejection_comment validated together, after both are merged,
	// since "rejected requires a comment" depends on the final value of
	// both fields regardless of which one(s) this call actually changed.
	if patch.Status != nil {
		listing.Status = *patch.Status
		touch("status")
	}
	if patch.RejectionComment.Set {
		listing.RejectionComment = patch.RejectionComment.Value
		touch("rejection_comment")
	}
	if patch.Status != nil || patch.RejectionComment.Set {
		if validationError := validateListingStatus(listing.Status, listing.RejectionComment); validationError != nil {
			return validationError
		}
	}

	return nil
}

// Delete removes a listing's image directory before removing its DB row
// (ADR 0012 delete order — if file removal fails, the DB row is kept, so
// there's never an orphaned file with no DB reference). listing_image /
// listing_criterion_grade / listing_note rows cascade via FK once the row
// is gone.
func (service *ListingService) Delete(ctx context.Context, id uuid.UUID) error {
	imageDir := filepath.Join(service.dataDir, "images", "listings", id.String())
	if err := os.RemoveAll(imageDir); err != nil {
		return fmt.Errorf("remove listing image directory: %w", err)
	}
	return repository.NewListingRepo(service.pool).Delete(ctx, id)
}

func validateNonNegative(field string, value *float64) error {
	if value != nil && *value < 0 {
		return &domain.ValidationError{Field: field, Message: field + " darf nicht negativ sein"}
	}
	return nil
}

func validateListingStatus(status domain.ListingStatus, rejectionComment *string) error {
	if !validListingStatuses[status] {
		return &domain.ValidationError{Field: "status", Message: "Ungültiger Status: " + string(status)}
	}
	if status == domain.ListingStatusRejected && (rejectionComment == nil || strings.TrimSpace(*rejectionComment) == "") {
		return &domain.ValidationError{Field: "rejection_comment", Message: "Ablehnung erfordert einen Kommentar"}
	}
	return nil
}

func uniqueAppend(fields []string, field string) []string {
	if slices.Contains(fields, field) {
		return fields
	}
	return append(fields, field)
}
