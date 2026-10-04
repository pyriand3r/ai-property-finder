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

// jpegMagicBytes / pngMagicBytes are the fixed headers every JPEG/PNG
// file starts with.
var jpegMagicBytes = []byte{0xFF, 0xD8, 0xFF}
var pngMagicBytes = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

// UploadService stores generic file uploads that aren't yet tied to a
// domain entity — PDFs and JPG/PNG images (spec 00-03 Layer 3.3, ADR
// 0020). The file is later consumed by the chat/Agent ingestion flow
// (internal/attachment, Layer 6+), which deletes it once the ingestion
// turn completes.
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

// SaveImage validates file starts with a JPEG or PNG magic header and
// writes it to {DataDir}/uploads/image/{uuid}.{jpg,png}, returning the
// path relative to DataDir (same convention as SavePDF).
func (service *UploadService) SaveImage(_ context.Context, file io.Reader) (string, error) {
	head := make([]byte, len(pngMagicBytes)) // longest of the two signatures
	n, readError := io.ReadFull(file, head)
	if readError != nil && readError != io.ErrUnexpectedEOF && readError != io.EOF {
		return "", fmt.Errorf("read uploaded image: %w", readError)
	}
	head = head[:n]

	var ext string
	switch {
	case bytes.HasPrefix(head, jpegMagicBytes):
		ext = ".jpg"
	case bytes.HasPrefix(head, pngMagicBytes):
		ext = ".png"
	default:
		return "", &domain.ValidationError{Field: "file", Message: "Datei ist kein gültiges JPG oder PNG"}
	}

	dir := filepath.Join(service.dataDir, "uploads", "image")
	if mkdirError := os.MkdirAll(dir, 0o750); mkdirError != nil {
		return "", fmt.Errorf("create upload directory: %w", mkdirError)
	}

	relPath := filepath.Join("uploads", "image", uuid.New().String()+ext)
	absPath := filepath.Join(service.dataDir, relPath)

	dest, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return "", fmt.Errorf("create image file: %w", err)
	}
	defer dest.Close()

	if _, err := dest.Write(head); err != nil {
		_ = os.Remove(absPath)
		return "", fmt.Errorf("write image file: %w", err)
	}
	if _, err := io.Copy(dest, file); err != nil {
		_ = os.Remove(absPath)
		return "", fmt.Errorf("write image file: %w", err)
	}

	return filepath.ToSlash(relPath), nil
}
