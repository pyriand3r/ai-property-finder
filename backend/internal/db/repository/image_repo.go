package repository

import (
	"context"
	"errors"
	"fmt"
	"propertyfinder/internal/db"
	"propertyfinder/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ImageRepository persists the listing_image aggregate.
type ImageRepository interface {
	Create(ctx context.Context, image *domain.ListingImage) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ListingImage, error)
	ListByListing(ctx context.Context, listingID uuid.UUID) ([]domain.ListingImage, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// CountByListing is used to assign a new image's sort_order (append at
	// the end) and to detect a listing's first upload (Layer 3.2).
	CountByListing(ctx context.Context, listingID uuid.UUID) (int, error)
}

// ImageRepo is the pgx-backed implementation of ImageRepository.
type ImageRepo struct {
	db db.Querier
}

// NewImageRepo constructs an ImageRepo wired to q — either the pool or a
// transaction.
func NewImageRepo(q db.Querier) *ImageRepo {
	return &ImageRepo{db: q}
}

const imageColumns = `id, listing_id, url, is_local, sort_order, file_path, category, created_at`

func (repo *ImageRepo) Create(ctx context.Context, image *domain.ListingImage) error {
	query := `INSERT INTO listing_image (` + imageColumns + `) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := repo.db.Exec(ctx, query,
		image.ID,
		image.ListingID,
		image.URL,
		image.IsLocal,
		image.SortOrder,
		image.FilePath,
		image.Category,
		image.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("image create: %w", err)
	}
	return nil
}

func (repo *ImageRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.ListingImage, error) {
	query := `SELECT ` + imageColumns + ` FROM listing_image WHERE id = $1`
	row := repo.db.QueryRow(ctx, query, id)
	image, err := scanImageRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "listing_image", ID: id.String()}
		}
		return nil, fmt.Errorf("image get: %w", err)
	}
	return &image, nil
}

func (repo *ImageRepo) ListByListing(ctx context.Context, listingID uuid.UUID) ([]domain.ListingImage, error) {
	query := `SELECT ` + imageColumns + ` FROM listing_image WHERE listing_id = $1 ORDER BY sort_order`
	rows, err := repo.db.Query(ctx, query, listingID)
	if err != nil {
		return nil, fmt.Errorf("image list: %w", err)
	}
	defer rows.Close()

	images := make([]domain.ListingImage, 0)
	for rows.Next() {
		image, scanError := scanImageRow(rows)
		if scanError != nil {
			return nil, fmt.Errorf("image list scan: %w", scanError)
		}
		images = append(images, image)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("image list rows: %w", rowsError)
	}
	return images, nil
}

func (repo *ImageRepo) Delete(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM listing_image WHERE id = $1`
	tag, err := repo.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("image delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "listing_image", ID: id.String()}
	}
	return nil
}

func (repo *ImageRepo) CountByListing(ctx context.Context, listingID uuid.UUID) (int, error) {
	const query = `SELECT COUNT(*) FROM listing_image WHERE listing_id = $1`
	var count int
	if err := repo.db.QueryRow(ctx, query, listingID).Scan(&count); err != nil {
		return 0, fmt.Errorf("image count by listing: %w", err)
	}
	return count, nil
}

func scanImageRow(scanner interface {
	Scan(dest ...any) error
},
) (domain.ListingImage, error) {
	var image domain.ListingImage
	err := scanner.Scan(
		&image.ID,
		&image.ListingID,
		&image.URL,
		&image.IsLocal,
		&image.SortOrder,
		&image.FilePath,
		&image.Category,
		&image.CreatedAt,
	)
	return image, err
}
