package api

import (
	"net/http"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"
)

// maxPDFUploadBytes matches Claude's document size limit (spec 00-03
// "PDF Handling").
const maxPDFUploadBytes = 32 << 20 // 32 MB

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
