package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"propertyfinder/internal/db"
	"propertyfinder/internal/domain"

	"github.com/jackc/pgx/v5"
)

// HazardRepository persists the static hazard tables (docs/adr/0025). The
// Upsert and Prune methods are the dataset sync's write path (called by
// service.HazardService at boot, inside one transaction); the rest serve
// reads. No request ever writes these tables.
type HazardRepository interface {
	// UpsertSources inserts or updates sources by id.
	UpsertSources(ctx context.Context, sources []domain.HazardSource) error
	// UpsertRegions inserts or updates regions by key, including their
	// NameSearch column.
	UpsertRegions(ctx context.Context, regions []domain.HazardRegion) error
	// UpsertRatings inserts or updates ratings by (region, hazard type).
	// updated_at moves only when the rating or its source changed.
	UpsertRatings(ctx context.Context, ratings []domain.HazardRating) error
	// PruneRatings deletes the ratings of a country's regions that are not in
	// keep: a dataset update that drops a rating must not leave it behind.
	PruneRatings(ctx context.Context, country string, keep []domain.HazardRating) error
	// PruneRegions deletes a country's regions whose key is not in keepKeys
	// (their ratings go with them).
	PruneRegions(ctx context.Context, country string, keepKeys []string) error
	// PruneSources deletes sources no rating cites any more.
	PruneSources(ctx context.Context) error
	// SearchRegions lists regions ordered by name. country "" means any
	// country; nameSearch "" means any name, otherwise it must already be
	// normalized (hazard.NormalizeName) and is matched as a substring.
	SearchRegions(ctx context.Context, country, nameSearch string) ([]domain.HazardRegion, error)
	GetRegion(ctx context.Context, key string) (*domain.HazardRegion, error)
	// ListRatingsByRegion returns a region's ratings with their sources,
	// ordered by hazard type name.
	ListRatingsByRegion(ctx context.Context, regionKey string) ([]domain.HazardRatingDetail, error)
}

// HazardRepo is the pgx-backed implementation of HazardRepository.
type HazardRepo struct {
	db db.Querier
}

// NewHazardRepo constructs a HazardRepo wired to q — either the pool or a
// transaction.
func NewHazardRepo(q db.Querier) *HazardRepo {
	return &HazardRepo{db: q}
}

// The sync statements take their rows as one JSON array and expand it with
// jsonb_to_recordset: a dataset has hundreds of rows, and one statement per
// table keeps the boot-time sync to a handful of round trips.

// UpsertSources inserts or updates sources by id.
func (repo *HazardRepo) UpsertSources(ctx context.Context, sources []domain.HazardSource) error {
	const query = `
INSERT INTO hazard_source (id, hazard_type, title, publisher, url, license, dataset_version, retrieved_on, method, notes)
SELECT id, hazard_type, title, publisher, url, license, dataset_version, retrieved_on::date, method, notes
FROM jsonb_to_recordset($1::jsonb) AS t(
    id text, hazard_type text, title text, publisher text, url text, license text,
    dataset_version text, retrieved_on text, method text, notes text)
ON CONFLICT (id) DO UPDATE SET
    hazard_type     = EXCLUDED.hazard_type,
    title           = EXCLUDED.title,
    publisher       = EXCLUDED.publisher,
    url             = EXCLUDED.url,
    license         = EXCLUDED.license,
    dataset_version = EXCLUDED.dataset_version,
    retrieved_on    = EXCLUDED.retrieved_on,
    method          = EXCLUDED.method,
    notes           = EXCLUDED.notes
`
	return execRows(ctx, repo.db, "hazard upsert sources", query, sources)
}

// hazardRegionRow is a HazardRegion as the sync statement reads it:
// domain.HazardRegion hides NameSearch from clients, the table needs it.
type hazardRegionRow struct {
	Key        string `json:"key"`
	Country    string `json:"country"`
	Name       string `json:"name"`
	ParentName string `json:"parent_name"`
	NameSearch string `json:"name_search"`
}

// UpsertRegions inserts or updates regions by key.
func (repo *HazardRepo) UpsertRegions(ctx context.Context, regions []domain.HazardRegion) error {
	const query = `
INSERT INTO hazard_region (key, country, name, parent_name, name_search)
SELECT key, country, name, parent_name, name_search
FROM jsonb_to_recordset($1::jsonb) AS t(key text, country text, name text, parent_name text, name_search text)
ON CONFLICT (key) DO UPDATE SET
    country     = EXCLUDED.country,
    name        = EXCLUDED.name,
    parent_name = EXCLUDED.parent_name,
    name_search = EXCLUDED.name_search
`
	rows := make([]hazardRegionRow, len(regions))
	for i, region := range regions {
		rows[i] = hazardRegionRow{
			Key:        region.Key,
			Country:    region.Country,
			Name:       region.Name,
			ParentName: region.ParentName,
			NameSearch: region.NameSearch,
		}
	}
	return execRows(ctx, repo.db, "hazard upsert regions", query, rows)
}

// UpsertRatings inserts or updates ratings by (region, hazard type). The
// WHERE clause leaves an unchanged row alone, so updated_at records the last
// real change and not the last boot.
func (repo *HazardRepo) UpsertRatings(ctx context.Context, ratings []domain.HazardRating) error {
	const query = `
INSERT INTO hazard_rating (region_key, hazard_type, rating, source_id, updated_at)
SELECT region_key, hazard_type, rating, source_id, NOW()
FROM jsonb_to_recordset($1::jsonb) AS t(region_key text, hazard_type text, rating smallint, source_id text)
ON CONFLICT (region_key, hazard_type) DO UPDATE SET
    rating     = EXCLUDED.rating,
    source_id  = EXCLUDED.source_id,
    updated_at = NOW()
WHERE (hazard_rating.rating, hazard_rating.source_id) IS DISTINCT FROM (EXCLUDED.rating, EXCLUDED.source_id)
`
	return execRows(ctx, repo.db, "hazard upsert ratings", query, ratings)
}

// PruneRatings deletes the ratings of a country's regions that are not in
// keep.
func (repo *HazardRepo) PruneRatings(ctx context.Context, country string, keep []domain.HazardRating) error {
	const query = `
DELETE FROM hazard_rating r
USING hazard_region g
WHERE g.key = r.region_key
  AND g.country = $1
  AND NOT EXISTS (
      SELECT 1
      FROM jsonb_to_recordset($2::jsonb) AS k(region_key text, hazard_type text)
      WHERE k.region_key = r.region_key AND k.hazard_type = r.hazard_type)
`
	rows, err := marshalRows(keep)
	if err != nil {
		return fmt.Errorf("hazard prune ratings: %w", err)
	}
	if _, execError := repo.db.Exec(ctx, query, country, rows); execError != nil {
		return fmt.Errorf("hazard prune ratings: %w", execError)
	}
	return nil
}

// PruneRegions deletes a country's regions whose key is not in keepKeys.
func (repo *HazardRepo) PruneRegions(ctx context.Context, country string, keepKeys []string) error {
	const query = `DELETE FROM hazard_region WHERE country = $1 AND key <> ALL($2::text[])`
	if keepKeys == nil {
		keepKeys = []string{} // a nil slice is SQL NULL, and "<> ALL(NULL)" deletes nothing
	}
	if _, err := repo.db.Exec(ctx, query, country, keepKeys); err != nil {
		return fmt.Errorf("hazard prune regions: %w", err)
	}
	return nil
}

// PruneSources deletes sources no rating cites any more.
func (repo *HazardRepo) PruneSources(ctx context.Context) error {
	const query = `
DELETE FROM hazard_source s
WHERE NOT EXISTS (SELECT 1 FROM hazard_rating r WHERE r.source_id = s.id)
`
	if _, err := repo.db.Exec(ctx, query); err != nil {
		return fmt.Errorf("hazard prune sources: %w", err)
	}
	return nil
}

// SearchRegions lists regions ordered by name; see HazardRepository.
func (repo *HazardRepo) SearchRegions(ctx context.Context, country, nameSearch string) ([]domain.HazardRegion, error) {
	// strpos, not LIKE: a "%" or "_" typed by the caller must not act as a wildcard.
	const query = `
SELECT key, country, name, parent_name
FROM hazard_region
WHERE ($1::text = '' OR country = $1::text)
  AND ($2::text = '' OR strpos(name_search, $2::text) > 0)
ORDER BY name_search, key
`
	rows, err := repo.db.Query(ctx, query, country, nameSearch)
	if err != nil {
		return nil, fmt.Errorf("hazard search regions: %w", err)
	}
	defer rows.Close()

	regions := make([]domain.HazardRegion, 0)
	for rows.Next() {
		region, scanError := scanHazardRegionRow(rows)
		if scanError != nil {
			return nil, fmt.Errorf("hazard search regions scan: %w", scanError)
		}
		regions = append(regions, region)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("hazard search regions rows: %w", rowsError)
	}
	return regions, nil
}

// GetRegion returns one region by key.
func (repo *HazardRepo) GetRegion(ctx context.Context, key string) (*domain.HazardRegion, error) {
	const query = `SELECT key, country, name, parent_name FROM hazard_region WHERE key = $1`
	region, err := scanHazardRegionRow(repo.db.QueryRow(ctx, query, key))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Entity: "hazard region", ID: key}
		}
		return nil, fmt.Errorf("hazard get region: %w", err)
	}
	return &region, nil
}

// ListRatingsByRegion returns a region's ratings with their sources.
func (repo *HazardRepo) ListRatingsByRegion(ctx context.Context, regionKey string) ([]domain.HazardRatingDetail, error) {
	const query = `
SELECT r.hazard_type, r.rating, r.updated_at,
       s.id, s.hazard_type, s.title, s.publisher, s.url, s.license, s.dataset_version,
       to_char(s.retrieved_on, 'YYYY-MM-DD'), s.method, s.notes
FROM hazard_rating r
JOIN hazard_source s ON s.id = r.source_id AND s.hazard_type = r.hazard_type
WHERE r.region_key = $1
ORDER BY r.hazard_type
`
	rows, err := repo.db.Query(ctx, query, regionKey)
	if err != nil {
		return nil, fmt.Errorf("hazard list ratings: %w", err)
	}
	defer rows.Close()

	ratings := make([]domain.HazardRatingDetail, 0)
	for rows.Next() {
		var detail domain.HazardRatingDetail
		scanError := rows.Scan(
			&detail.HazardType,
			&detail.Rating,
			&detail.UpdatedAt,
			&detail.Source.ID,
			&detail.Source.HazardType,
			&detail.Source.Title,
			&detail.Source.Publisher,
			&detail.Source.URL,
			&detail.Source.License,
			&detail.Source.DatasetVersion,
			&detail.Source.RetrievedOn,
			&detail.Source.Method,
			&detail.Source.Notes,
		)
		if scanError != nil {
			return nil, fmt.Errorf("hazard list ratings scan: %w", scanError)
		}
		ratings = append(ratings, detail)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("hazard list ratings rows: %w", rowsError)
	}
	return ratings, nil
}

// execRows runs query with rows as its one JSON-array argument. An empty
// list is a no-op: there is nothing to write.
func execRows[T any](ctx context.Context, q db.Querier, op, query string, rows []T) error {
	if len(rows) == 0 {
		return nil
	}
	payload, err := marshalRows(rows)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if _, execError := q.Exec(ctx, query, payload); execError != nil {
		return fmt.Errorf("%s: %w", op, execError)
	}
	return nil
}

// marshalRows encodes rows as a JSON array; a nil slice becomes [] (not
// null), which jsonb_to_recordset reads as "no rows".
func marshalRows[T any](rows []T) (json.RawMessage, error) {
	if rows == nil {
		rows = []T{}
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return nil, fmt.Errorf("marshal rows: %w", err)
	}
	return payload, nil
}

func scanHazardRegionRow(scanner interface {
	Scan(dest ...any) error
},
) (domain.HazardRegion, error) {
	var region domain.HazardRegion
	err := scanner.Scan(&region.Key, &region.Country, &region.Name, &region.ParentName)
	return region, err
}
