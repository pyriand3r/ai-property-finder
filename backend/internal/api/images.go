package api

import (
	"net/http"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"
	"strings"
)

// maxImageUploadBytes caps a single image upload. Generous for real-estate
// photos straight off a phone camera without being unbounded.
const maxImageUploadBytes = 20 << 20 // 20 MB

// ImageHandler serves the listing image endpoints (spec 01-07/ADR 0012
// Layer 3.1).
type ImageHandler struct {
	service *service.ImageService
}

// Upload handles POST /api/listings/{listingId}/images (multipart/form-data,
// field "file"; optional field "category").
func (handler *ImageHandler) Upload(writer http.ResponseWriter, request *http.Request) {
	listingID, ok := parseUUID(writer, request, "listingId")
	if !ok {
		return
	}

	request.Body = http.MaxBytesReader(writer, request.Body, maxImageUploadBytes)
	file, header, err := request.FormFile("file")
	if err != nil {
		writeError(writer, &domain.ValidationError{Field: "file", Message: "Datei fehlt oder ist zu groß (max 20 MB)"})
		return
	}
	defer file.Close()

	var category *string
	if raw := strings.TrimSpace(request.FormValue("category")); raw != "" {
		category = &raw
	}

	image, uploadError := handler.service.Upload(request.Context(), listingID, file, header.Filename, category)
	if uploadError != nil {
		writeError(writer, uploadError)
		return
	}
	writeJSON(writer, http.StatusCreated, image)
}

// List handles GET /api/listings/{listingId}/images.
func (handler *ImageHandler) List(writer http.ResponseWriter, request *http.Request) {
	listingID, ok := parseUUID(writer, request, "listingId")
	if !ok {
		return
	}

	images, err := handler.service.List(request.Context(), listingID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, images)
}

// Delete handles DELETE /api/listings/{listingId}/images/{imageId}. The
// listingId in the path is not otherwise used (image IDs are globally
// unique) but keeps the URL RESTfully scoped, matching the plan.
func (handler *ImageHandler) Delete(writer http.ResponseWriter, request *http.Request) {
	imageID, ok := parseUUID(writer, request, "imageId")
	if !ok {
		return
	}

	if err := handler.service.Delete(request.Context(), imageID); err != nil {
		writeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
