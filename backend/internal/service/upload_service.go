package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"propertyfinder/internal/domain"

	"github.com/google/uuid"
)

// pdfMagicBytes is the fixed header every PDF file starts with.
var pdfMagicBytes = []byte("%PDF-")

// UploadService stores generic file uploads that aren't yet tied to a
// domain entity — currently just PDFs (spec 00-03 Layer 3.3). The file is
// later consumed by the chat/Agent PDF-ingestion flow (internal/pdf,
// Layer 6+), which deletes it once the ingestion turn completes.
type UploadService struct {
	dataDir string
}

// NewUploadService constructs an UploadService wired to dataDir.
func NewUploadService(dataDir string) *UploadService {
	return &UploadService{dataDir: dataDir}
}

// SavePDF validates file starts with the PDF magic header and writes it to
// {DataDir}/uploads/pdf/{uuid}.pdf, returning the path relative to
// DataDir (same convention as listing_image.file_path).
func (service *UploadService) SavePDF(_ context.Context, file io.Reader) (string, error) {
	head := make([]byte, len(pdfMagicBytes))
	n, readError := io.ReadFull(file, head)
	if readError != nil && readError != io.ErrUnexpectedEOF && readError != io.EOF {
		return "", fmt.Errorf("read uploaded pdf: %w", readError)
	}
	head = head[:n]
	if !bytes.Equal(head, pdfMagicBytes) {
		return "", &domain.ValidationError{Field: "file", Message: "Datei ist kein gültiges PDF"}
	}

	dir := filepath.Join(service.dataDir, "uploads", "pdf")
	if mkdirError := os.MkdirAll(dir, 0o750); mkdirError != nil {
		return "", fmt.Errorf("create upload directory: %w", mkdirError)
	}

	relPath := filepath.Join("uploads", "pdf", uuid.New().String()+".pdf")
	absPath := filepath.Join(service.dataDir, relPath)

	dest, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return "", fmt.Errorf("create pdf file: %w", err)
	}
	defer dest.Close()

	if _, err := dest.Write(head); err != nil {
		_ = os.Remove(absPath)
		return "", fmt.Errorf("write pdf file: %w", err)
	}
	if _, err := io.Copy(dest, file); err != nil {
		_ = os.Remove(absPath)
		return "", fmt.Errorf("write pdf file: %w", err)
	}

	return filepath.ToSlash(relPath), nil
}
