package service

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveImage_WritesJPEGWithJPGExtension(t *testing.T) {
	dataDir := t.TempDir()
	service := NewUploadService(dataDir)
	data := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("fake jpeg body")...)

	path, err := service.SaveImage(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("SaveImage returned error: %v", err)
	}
	if filepath.Ext(path) != ".jpg" {
		t.Errorf("path = %q, want .jpg extension", path)
	}
	written, readErr := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(path)))
	if readErr != nil {
		t.Fatalf("file not written: %v", readErr)
	}
	if !bytes.Equal(written, data) {
		t.Errorf("written content = %q, want %q", written, data)
	}
}

func TestSaveImage_WritesPNGWithPNGExtension(t *testing.T) {
	dataDir := t.TempDir()
	service := NewUploadService(dataDir)
	data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, []byte("fake png body")...)

	path, err := service.SaveImage(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("SaveImage returned error: %v", err)
	}
	if filepath.Ext(path) != ".png" {
		t.Errorf("path = %q, want .png extension", path)
	}
}

func TestSaveImage_RejectsNonImageContent(t *testing.T) {
	service := NewUploadService(t.TempDir())
	if _, err := service.SaveImage(context.Background(), bytes.NewReader([]byte("not an image"))); err == nil {
		t.Error("expected an error for non-image content")
	}
}
