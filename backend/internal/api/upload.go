package api

import (
	"net/http"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"
)

// maxPDFUploadBytes matches Claude's document size limit (spec 00-03
// "PDF Handling").
const maxPDFUploadBytes = 32 << 20 // 32 MB

// maxChatImageUploadBytes is 3.75 MB of raw bytes: base64 encoding
// inflates by 4/3, so this keeps the encoded payload within Claude's 5 MB
// image input limit (platform.claude.com/docs vision). Deliberately a
// separate, smaller constant from images.go's maxImageUploadBytes (listing
// photo uploads, a different endpoint with a different purpose).
const maxChatImageUploadBytes = 3_932_160

// UploadHandler serves generic file upload endpoints not yet tied to a
// domain entity (spec 00-03 Layer 3.3).
type UploadHandler struct {
	service *service.UploadService
}

type uploadResponse struct {
	Path string `json:"path"`
}

// PDF handles POST /api/upload/pdf (multipart/form-data, field "file").
func (handler *UploadHandler) PDF(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, maxPDFUploadBytes)
	file, _, err := request.FormFile("file")
	if err != nil {
		writeError(writer, &domain.ValidationError{Field: "file", Message: "Datei fehlt oder ist zu groß (max 32 MB)"})
		return
	}
	defer file.Close()

	path, saveError := handler.service.SavePDF(request.Context(), file)
	if saveError != nil {
		writeError(writer, saveError)
		return
	}
	writeJSON(writer, http.StatusCreated, uploadResponse{Path: path})
}

// Image handles POST /api/upload/image (multipart/form-data, field "file").
func (handler *UploadHandler) Image(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, maxChatImageUploadBytes)
	file, _, err := request.FormFile("file")
	if err != nil {
		writeError(writer, &domain.ValidationError{Field: "file", Message: "Datei fehlt oder ist zu groß (max 3,75 MB)"})
		return
	}
	defer file.Close()

	path, saveError := handler.service.SaveImage(request.Context(), file)
	if saveError != nil {
		writeError(writer, saveError)
		return
	}
	writeJSON(writer, http.StatusCreated, uploadResponse{Path: path})
}
