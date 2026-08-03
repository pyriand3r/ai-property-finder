// Package pdf reads/removes the uploaded PDFs the Agent attaches natively
// to a chat turn (spec 00-03 "PDF Handling"). It has no opinion about
// Claude's document content block shape — that translation happens in
// internal/agent/provider/claude — this package only touches the
// filesystem.
package pdf

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

// ReadBase64 reads the PDF at dataDir/relativePath and returns its
// contents base64-encoded, ready for a native Claude document content
// block. relativePath is the value stored in chat_message.attachment_path
// (same convention as listing_image.file_path — relative to DataDir).
func ReadBase64(dataDir, relativePath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(relativePath)))
	if err != nil {
		return "", fmt.Errorf("read pdf %s: %w", relativePath, err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// Delete removes the PDF at dataDir/relativePath once its ingestion turn
// has completed — spec 00-03: "the PDF file is deleted from disk... not
// resent in subsequent turns, this prevents silent token consumption
// across sessions." Deleting an already-gone file is not an error, so a
// turn that fails after the read but before cleanup doesn't leave a
// second attempt looking broken.
func Delete(dataDir, relativePath string) error {
	err := os.Remove(filepath.Join(dataDir, filepath.FromSlash(relativePath)))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete pdf %s: %w", relativePath, err)
	}
	return nil
}
