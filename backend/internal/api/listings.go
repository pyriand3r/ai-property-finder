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

// listingPatchBody is the wire shape of a listing create/PATCH body. An
// omitted key means "unchanged". The nullable fields use service.Optional so
// an explicit JSON null can be told apart from an omitted key and clears the
// field (issue #9). title, status and title_image_id stay plain pointers:
// they are not clearable, and null on them is treated as omitted.
type listingPatchBody struct {
	Title            *string                     `json:"title,omitempty"`
	SourceURL        service.Optional[string]    `json:"source_url"`
	Price            service.Optional[float64]   `json:"price"`
	PriceCurrency    service.Optional[string]    `json:"price_currency"`
	PropertySizeM2   service.Optional[float64]   `json:"property_size_m2"`
	PlotSizeM2       service.Optional[float64]   `json:"plot_size_m2"`
	Address          service.Optional[string]    `json:"address"`
	Region           service.Optional[string]    `json:"region"`
	Country          service.Optional[string]    `json:"country"`
	Latitude         service.Optional[float64]   `json:"latitude"`
	Longitude        service.Optional[float64]   `json:"longitude"`
	Description      service.Optional[string]    `json:"description"`
	TitleImagePath   service.Optional[string]    `json:"title_image_path"`
	TitleImageID     *uuid.UUID                  `json:"title_image_id,omitempty"`
	Status           *domain.ListingStatus       `json:"status,omitempty"`
	RejectionComment service.Optional[string]    `json:"rejection_comment"`
	SellerName       service.Optional[string]    `json:"seller_name"`
	SellerPhone      service.Optional[string]    `json:"seller_phone"`
	SellerEmail      service.Optional[string]    `json:"seller_email"`
	SellerNotes      service.Optional[string]    `json:"seller_notes"`
	LastContactAt    service.Optional[time.Time] `json:"last_contact_at"`
	DocumentMD       service.Optional[string]    `json:"document_md"`
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

// Create handles POST /api/profiles/{profileId}/listings. A listing whose
// address or coordinates match an existing one in the profile is answered
// with 409 and the matches, unless ?confirm_duplicate=true is set (issue #10).
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

	confirmDuplicate := request.URL.Query().Get("confirm_duplicate") == "true"

	listing, err := handler.service.Create(request.Context(), profileID, title, body.toPatch(), confirmDuplicate)
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
