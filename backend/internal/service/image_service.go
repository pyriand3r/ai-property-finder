package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var validImageCategories = map[string]bool{
	"exterior":    true,
	"interior":    true,
	"floor_plan":  true,
	"outbuilding": true,
	"other":       true,
}

// imageExtensionsByContentType maps sniffed MIME types to a file extension,
// used when the uploaded filename has no (or an unrecognized) extension.
var imageExtensionsByContentType = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

var validImageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

// ImageService stores and serves Listing photos under
// {DataDir}/images/listings/{listing_id}/{uuid}.{ext} (ADR 0012).
type ImageService struct {
	pool    *pgxpool.Pool
	dataDir string
}

// NewImageService constructs an ImageService wired to pool and dataDir.
func NewImageService(pool *pgxpool.Pool, dataDir string) *ImageService {
	return &ImageService{pool: pool, dataDir: dataDir}
}

// Upload validates listingID exists, determines a safe extension from
// filename/content, writes the file to disk under a fresh UUID name, and
// inserts the listing_image row appended to the end of the listing's
// sort_order.
func (service *ImageService) Upload(ctx context.Context, listingID uuid.UUID, file io.Reader, filename string, category *string) (*domain.ListingImage, error) {
	if _, err := repository.NewListingRepo(service.pool).GetByID(ctx, listingID); err != nil {
		return nil, err
	}
	if category != nil && !validImageCategories[*category] {
		return nil, &domain.ValidationError{Field: "category", Message: "unbekannte Kategorie: " + *category}
	}

	imageRepo := repository.NewImageRepo(service.pool)
	count, err := imageRepo.CountByListing(ctx, listingID)
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(service.dataDir, "images", "listings", listingID.String())
	if mkdirError := os.MkdirAll(dir, 0o750); mkdirError != nil {
		return nil, fmt.Errorf("create image directory: %w", mkdirError)
	}

	imageID := uuid.New()
	buffered := make([]byte, 512)
	n, readError := io.ReadFull(file, buffered)
	if readError != nil && readError != io.ErrUnexpectedEOF && readError != io.EOF {
		return nil, fmt.Errorf("read uploaded image: %w", readError)
	}
	buffered = buffered[:n]

	ext := safeImageExtension(filename, buffered)
	if ext == "" {
		return nil, &domain.ValidationError{Field: "file", Message: "nicht erkanntes Bildformat (unterstützt: jpg, png, webp, gif)"}
	}

	relPath := filepath.Join("images", "listings", listingID.String(), imageID.String()+ext)
	absPath := filepath.Join(service.dataDir, relPath)

	dest, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return nil, fmt.Errorf("create image file: %w", err)
	}
	defer dest.Close()

	if _, err := dest.Write(buffered); err != nil {
		_ = os.Remove(absPath)
		return nil, fmt.Errorf("write image file: %w", err)
	}
	if _, err := io.Copy(dest, file); err != nil {
		_ = os.Remove(absPath)
		return nil, fmt.Errorf("write image file: %w", err)
	}

	image := &domain.ListingImage{
		ID:        imageID,
		ListingID: listingID,
		URL:       "",
		IsLocal:   true,
		SortOrder: count,
		FilePath:  filepath.ToSlash(relPath),
		Category:  category,
		CreatedAt: time.Now().UTC(),
	}
	if createError := imageRepo.Create(ctx, image); createError != nil {
		_ = os.Remove(absPath)
		return nil, createError
	}
	return image, nil
}

// List returns a listing's images ordered by sort_order.
func (service *ImageService) List(ctx context.Context, listingID uuid.UUID) ([]domain.ListingImage, error) {
	return repository.NewImageRepo(service.pool).ListByListing(ctx, listingID)
}

// Delete removes the image file, then its DB row. listing.title_image_id
// clears automatically via ON DELETE SET NULL if this was the title image.
func (service *ImageService) Delete(ctx context.Context, imageID uuid.UUID) error {
	imageRepo := repository.NewImageRepo(service.pool)
	image, err := imageRepo.GetByID(ctx, imageID)
	if err != nil {
		return err
	}
	if removeError := os.Remove(filepath.Join(service.dataDir, filepath.FromSlash(image.FilePath))); removeError != nil && !os.IsNotExist(removeError) {
		return fmt.Errorf("remove image file: %w", removeError)
	}
	return imageRepo.Delete(ctx, imageID)
}

// safeImageExtension returns a whitelisted, lowercased extension (with
// leading dot) for filename, falling back to sniffing head's content type
// when the filename has no recognized image extension. Returns "" when
// neither source yields a supported format — the original filename is
// otherwise discarded entirely (UUID-renamed), so this is the only
// path-traversal-relevant input and it never reaches the filesystem.
func safeImageExtension(filename string, head []byte) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if validImageExtensions[ext] {
		if ext == ".jpeg" {
			return ".jpg"
		}
		return ext
	}
	contentType := http.DetectContentType(head)
	if mapped, ok := imageExtensionsByContentType[strings.SplitN(contentType, ";", 2)[0]]; ok {
		return mapped
	}
	return ""
}
