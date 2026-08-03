package pdf

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestReadBase64_RoundTrips(t *testing.T) {
	dataDir := t.TempDir()
	relPath := filepath.Join("uploads", "pdf", "test.pdf")
	absPath := filepath.Join(dataDir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o750); err != nil {
		t.Fatalf("setup: %v", err)
	}
	want := []byte("%PDF-1.4 fake content")
	if err := os.WriteFile(absPath, want, 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	encoded, err := ReadBase64(dataDir, relPath)
	if err != nil {
		t.Fatalf("ReadBase64 returned error: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("result is not valid base64: %v", err)
	}
	if !bytes.Equal(decoded, want) {
		t.Errorf("decoded = %q, want %q", decoded, want)
	}
}

func TestReadBase64_MissingFile(t *testing.T) {
	if _, err := ReadBase64(t.TempDir(), "uploads/pdf/missing.pdf"); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestDelete_RemovesFile(t *testing.T) {
	dataDir := t.TempDir()
	relPath := "uploads/pdf/test.pdf"
	absPath := filepath.Join(dataDir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o750); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(absPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := Delete(dataDir, relPath); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := os.Stat(absPath); !os.IsNotExist(err) {
		t.Error("file still exists after Delete")
	}
}

func TestDelete_MissingFileIsNotAnError(t *testing.T) {
	if err := Delete(t.TempDir(), "uploads/pdf/missing.pdf"); err != nil {
		t.Errorf("Delete on a missing file returned an error: %v", err)
	}
}
