package api

import (
	"net/http"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// ListingHandler serves the listing endpoints (spec 01-07 Layer 2.1).
type ListingHandler struct {
	service *service.ListingService
}

// listingPatchBody is the wire shape of a listing create/PATCH body. All
// fields are pointers so an omitted field means "unchanged", matching the
// rest of the API (see profiles.go, criteria.go).
type listingPatchBody struct {
	Title            *string               `json:"title,omitempty"`
	SourceURL        *string               `json:"source_url,omitempty"`
	Price            *float64              `json:"price,omitempty"`
	PriceCurrency    *string               `json:"price_currency,omitempty"`
	PropertySizeM2   *float64              `json:"property_size_m2,omitempty"`
	PlotSizeM2       *float64              `json:"plot_size_m2,omitempty"`
	Address          *string               `json:"address,omitempty"`
	Region           *string               `json:"region,omitempty"`
	Country          *string               `json:"country,omitempty"`
	Latitude         *float64              `json:"latitude,omitempty"`
	Longitude        *float64              `json:"longitude,omitempty"`
	Description      *string               `json:"description,omitempty"`
	TitleImagePath   *string               `json:"title_image_path,omitempty"`
	TitleImageID     *uuid.UUID            `json:"title_image_id,omitempty"`
	Status           *domain.ListingStatus `json:"status,omitempty"`
	RejectionComment *string               `json:"rejection_comment,omitempty"`
	SellerName       *string               `json:"seller_name,omitempty"`
	SellerPhone      *string               `json:"seller_phone,omitempty"`
	SellerEmail      *string               `json:"seller_email,omitempty"`
	SellerNotes      *string               `json:"seller_notes,omitempty"`
	LastContactAt    *time.Time            `json:"last_contact_at,omitempty"`
	DocumentMD       *string               `json:"document_md,omitempty"`
}

func (body listingPatchBody) toPatch() service.ListingPatch {
	return service.ListingPatch{
		Title:            body.Title,
		SourceURL:        body.SourceURL,
		Price:            body.Price,
		PriceCurrency:    body.PriceCurrency,
		PropertySizeM2:   body.PropertySizeM2,
		PlotSizeM2:       body.PlotSizeM2,
		Address:          body.Address,
		Region:           body.Region,
		Country:          body.Country,
		Latitude:         body.Latitude,
		Longitude:        body.Longitude,
		Description:      body.Description,
		TitleImagePath:   body.TitleImagePath,
		TitleImageID:     body.TitleImageID,
		Status:           body.Status,
		RejectionComment: body.RejectionComment,
		SellerName:       body.SellerName,
		SellerPhone:      body.SellerPhone,
		SellerEmail:      body.SellerEmail,
		SellerNotes:      body.SellerNotes,
		LastContactAt:    body.LastContactAt,
		DocumentMD:       body.DocumentMD,
	}
}

// List handles GET /api/profiles/{profileId}/listings, optionally filtered
// by ?needs_reevaluation=true|false.
func (handler *ListingHandler) List(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	var needsReevaluation *bool
	if raw := request.URL.Query().Get("needs_reevaluation"); raw != "" {
		value, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			writeError(writer, &domain.ValidationError{Field: "needs_reevaluation", Message: "muss true oder false sein"})
			return
		}
		needsReevaluation = &value
	}

	listings, err := handler.service.List(request.Context(), profileID, needsReevaluation)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, listings)
}

// Create handles POST /api/profiles/{profileId}/listings.
func (handler *ListingHandler) Create(writer http.ResponseWriter, request *http.Request) {
	profileID, ok := parseUUID(writer, request, "profileId")
	if !ok {
		return
	}

	var body listingPatchBody
	if !decodeJSON(writer, request, &body) {
		return
	}
	var title string
	if body.Title != nil {
		title = *body.Title
	}

	listing, err := handler.service.Create(request.Context(), profileID, title, body.toPatch())
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, listing)
}

// Get handles GET /api/listings/{listingId}.
func (handler *ListingHandler) Get(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseUUID(writer, request, "listingId")
	if !ok {
		return
	}

	listing, err := handler.service.Get(request.Context(), id)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, listing)
}

// Update handles PATCH /api/listings/{listingId}.
func (handler *ListingHandler) Update(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseUUID(writer, request, "listingId")
	if !ok {
		return
	}

	var body listingPatchBody
	if !decodeJSON(writer, request, &body) {
		return
	}

	listing, err := handler.service.Update(request.Context(), id, body.toPatch())
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, listing)
}

// Delete handles DELETE /api/listings/{listingId}.
func (handler *ListingHandler) Delete(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseUUID(writer, request, "listingId")
	if !ok {
		return
	}

	if err := handler.service.Delete(request.Context(), id); err != nil {
		writeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
