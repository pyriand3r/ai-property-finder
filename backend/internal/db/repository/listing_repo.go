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

// ListingRepository persists the listing aggregate.
type ListingRepository interface {
	Create(ctx context.Context, listing *domain.Listing) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Listing, error)
	ListByProfile(ctx context.Context, profileID uuid.UUID) ([]domain.Listing, error)
	// Update overwrites every structured column from listing (a full
	// replace, not a partial patch — the service layer merges the partial
	// request onto the current row first).
	Update(ctx context.Context, listing *domain.Listing) error
	Delete(ctx context.Context, id uuid.UUID) error
	// RecomputeScores recalculates listing.overall_score for every listing
	// in profileID that has at least one Grade, using each criterion's
	// *current* weight. Grade letters map A=100 down to F=0 in steps of 20.
	// Listings with no grades yet are left untouched. Used on a weight-only
	// Criterion change (spec 01-04) — no LLM call, no needs_reevaluation
	// flag.
	RecomputeScores(ctx context.Context, profileID uuid.UUID) error
	// FlagNeedsReevaluation sets needs_reevaluation = true on every listing
	// in profileID. Used on any Criteria change other than weight-only
	// (add, remove, or a name/description/is_subjective edit).
	FlagNeedsReevaluation(ctx context.Context, profileID uuid.UUID) error
	// ClearNeedsReevaluation sets needs_reevaluation = false on a single
	// listing. Used after a successful evaluate_listing call (spec 01-04).
	ClearNeedsReevaluation(ctx context.Context, listingID uuid.UUID) error
}

// ListingRepo is the pgx-backed implementation of ListingRepository.
type ListingRepo struct {
	db db.Querier
}

// NewListingRepo constructs a ListingRepo wired to q — either the pool or a
// transaction.
func NewListingRepo(q db.Querier) *ListingRepo {
	return &ListingRepo{db: q}
}

const listingColumns = `
    id, profile_id, title, source_url, price, price_currency, property_size_m2, plot_size_m2,
    address, region, country, latitude, longitude, description, title_image_path, title_image_id,
    status, rejection_comment, overall_score, seller_name, seller_phone, seller_email, seller_notes,
    last_contact_at, document_md, source_type, source_file_path, user_confirmed_fields,
    needs_reevaluation, kurzfazit, created_at, updated_at
`

// Create inserts a new listing. The caller generates the UUID and
// timestamps and sets sensible defaults (status, source_type,
// user_confirmed_fields, needs_reevaluation).
func (repo *ListingRepo) Create(ctx context.Context, listing *domain.Listing) error {
	query := `INSERT INTO listing (` + listingColumns + `)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
        $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32)`
	_, err := repo.db.Exec(ctx, query, listingArgs(listing)...)
	if err != nil {
		return fmt.Errorf("listing create: %w", err)
	}
	return nil
}

// GetByID returns a single listing by its UUID.
func (repo *ListingRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Listing, error) {
	query := `SELECT ` + listingColumns + ` FROM listing WHERE id = $1`
	row := repo.db.QueryRow(ctx, query, id)
	listing, err := scanListingRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "listing", ID: id.String()}
		}
		return nil, fmt.Errorf("listing get: %w", err)
	}
	return &listing, nil
}

// ListByProfile returns all listings in profileID, most recently created
// first.
func (repo *ListingRepo) ListByProfile(ctx context.Context, profileID uuid.UUID) ([]domain.Listing, error) {
	query := `SELECT ` + listingColumns + ` FROM listing WHERE profile_id = $1 ORDER BY created_at DESC`
	rows, err := repo.db.Query(ctx, query, profileID)
	if err != nil {
		return nil, fmt.Errorf("listing list: %w", err)
	}
	defer rows.Close()

	listings := make([]domain.Listing, 0)
	for rows.Next() {
		listing, scanError := scanListingRow(rows)
		if scanError != nil {
			return nil, fmt.Errorf("listing list scan: %w", scanError)
		}
		listings = append(listings, listing)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("listing list rows: %w", rowsError)
	}
	return listings, nil
}

// Update overwrites every structured column and bumps updated_at.
func (repo *ListingRepo) Update(ctx context.Context, listing *domain.Listing) error {
	query := `
UPDATE listing SET
    title = $2, source_url = $3, price = $4, price_currency = $5, property_size_m2 = $6,
    plot_size_m2 = $7, address = $8, region = $9, country = $10, latitude = $11, longitude = $12,
    description = $13, title_image_path = $14, title_image_id = $15, status = $16,
    rejection_comment = $17, overall_score = $18, seller_name = $19, seller_phone = $20,
    seller_email = $21, seller_notes = $22, last_contact_at = $23, document_md = $24,
    source_type = $25, source_file_path = $26, user_confirmed_fields = $27,
    needs_reevaluation = $28, kurzfazit = $29, updated_at = $30
WHERE id = $1
`
	tag, err := repo.db.Exec(ctx, query,
		listing.ID,
		listing.Title,
		listing.SourceURL,
		listing.Price,
		listing.PriceCurrency,
		listing.PropertySizeM2,
		listing.PlotSizeM2,
		listing.Address,
		listing.Region,
		listing.Country,
		listing.Latitude,
		listing.Longitude,
		listing.Description,
		listing.TitleImagePath,
		listing.TitleImageID,
		listing.Status,
		listing.RejectionComment,
		listing.OverallScore,
		listing.SellerName,
		listing.SellerPhone,
		listing.SellerEmail,
		listing.SellerNotes,
		listing.LastContactAt,
		listing.DocumentMD,
		listing.SourceType,
		listing.SourceFilePath,
		listing.UserConfirmedFields,
		listing.NeedsReevaluation,
		listing.Kurzfazit,
		listing.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("listing update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "listing", ID: listing.ID.String()}
	}
	return nil
}

// Delete removes a listing by its UUID. The caller is responsible for
// removing its image files first (ADR 0012 delete order) — this only
// touches the DB row; listing_image/listing_criterion_grade/listing_note
// cascade via FK.
func (repo *ListingRepo) Delete(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM listing WHERE id = $1`
	tag, err := repo.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("listing delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Entity: "listing", ID: id.String()}
	}
	return nil
}

func (repo *ListingRepo) RecomputeScores(ctx context.Context, profileID uuid.UUID) error {
	const query = `
WITH grade_values AS (
    SELECT id, listing_id,
           CASE grade
               WHEN 'A' THEN 100 WHEN 'B' THEN 80 WHEN 'C' THEN 60
               WHEN 'D' THEN 40 WHEN 'E' THEN 20 WHEN 'F' THEN 0
           END AS value
    FROM listing_criterion_grade
),
scores AS (
    SELECT gv.listing_id,
           SUM(gv.value * c.weight)::numeric / NULLIF(SUM(c.weight), 0) AS score
    FROM grade_values gv
    JOIN listing_criterion_grade lcg ON lcg.id = gv.id
    JOIN criterion c ON c.id = lcg.criterion_id
    WHERE c.profile_id = $1
    GROUP BY gv.listing_id
)
UPDATE listing l
SET overall_score = scores.score, updated_at = NOW()
FROM scores
WHERE l.id = scores.listing_id
`
	if _, err := repo.db.Exec(ctx, query, profileID); err != nil {
		return fmt.Errorf("listing recompute scores: %w", err)
	}
	return nil
}

func (repo *ListingRepo) FlagNeedsReevaluation(ctx context.Context, profileID uuid.UUID) error {
	const query = `UPDATE listing SET needs_reevaluation = true, updated_at = NOW() WHERE profile_id = $1`
	if _, err := repo.db.Exec(ctx, query, profileID); err != nil {
		return fmt.Errorf("listing flag needs_reevaluation: %w", err)
	}
	return nil
}

func (repo *ListingRepo) ClearNeedsReevaluation(ctx context.Context, listingID uuid.UUID) error {
	const query = `UPDATE listing SET needs_reevaluation = false, updated_at = NOW() WHERE id = $1`
	if _, err := repo.db.Exec(ctx, query, listingID); err != nil {
		return fmt.Errorf("listing clear needs_reevaluation: %w", err)
	}
	return nil
}

func listingArgs(listing *domain.Listing) []any {
	return []any{
		listing.ID,
		listing.ProfileID,
		listing.Title,
		listing.SourceURL,
		listing.Price,
		listing.PriceCurrency,
		listing.PropertySizeM2,
		listing.PlotSizeM2,
		listing.Address,
		listing.Region,
		listing.Country,
		listing.Latitude,
		listing.Longitude,
		listing.Description,
		listing.TitleImagePath,
		listing.TitleImageID,
		listing.Status,
		listing.RejectionComment,
		listing.OverallScore,
		listing.SellerName,
		listing.SellerPhone,
		listing.SellerEmail,
		listing.SellerNotes,
		listing.LastContactAt,
		listing.DocumentMD,
		listing.SourceType,
		listing.SourceFilePath,
		listing.UserConfirmedFields,
		listing.NeedsReevaluation,
		listing.Kurzfazit,
		listing.CreatedAt,
		listing.UpdatedAt,
	}
}

func scanListingRow(scanner interface {
	Scan(dest ...any) error
},
) (domain.Listing, error) {
	var listing domain.Listing
	err := scanner.Scan(
		&listing.ID,
		&listing.ProfileID,
		&listing.Title,
		&listing.SourceURL,
		&listing.Price,
		&listing.PriceCurrency,
		&listing.PropertySizeM2,
		&listing.PlotSizeM2,
		&listing.Address,
		&listing.Region,
		&listing.Country,
		&listing.Latitude,
		&listing.Longitude,
		&listing.Description,
		&listing.TitleImagePath,
		&listing.TitleImageID,
		&listing.Status,
		&listing.RejectionComment,
		&listing.OverallScore,
		&listing.SellerName,
		&listing.SellerPhone,
		&listing.SellerEmail,
		&listing.SellerNotes,
		&listing.LastContactAt,
		&listing.DocumentMD,
		&listing.SourceType,
		&listing.SourceFilePath,
		&listing.UserConfirmedFields,
		&listing.NeedsReevaluation,
		&listing.Kurzfazit,
		&listing.CreatedAt,
		&listing.UpdatedAt,
	)
	return listing, err
}
