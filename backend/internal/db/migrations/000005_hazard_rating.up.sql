-- Region-level natural hazard ratings (docs/adr/0025, issue #44).
--
-- Static reference data: no request writes these tables. At boot the backend
-- syncs them from the dataset files embedded in the binary
-- (internal/hazard/datasets), so the tables are a queryable cache of those
-- files. They are independent of listing.overall_score and have no foreign
-- key to listing or search_profile.
--
-- hazard_type is TEXT + CHECK, like criterion.status (migration 000004), so a
-- new hazard type only needs one replaced constraint.

-- Where a rating comes from: one row per published dataset the ratings were
-- derived from. `method` says how the dataset's own classes were turned into
-- the 1..5 scale, so an app-specific interpretation is never mistaken for an
-- official one.
CREATE TABLE hazard_source (
    id              TEXT PRIMARY KEY,
    hazard_type     TEXT NOT NULL CHECK (hazard_type IN ('wildfire', 'flood', 'earthquake', 'landslide')),
    title           TEXT NOT NULL,
    publisher       TEXT NOT NULL,
    url             TEXT NOT NULL,
    license         TEXT NOT NULL,
    dataset_version TEXT NOT NULL,
    retrieved_on    DATE NOT NULL,
    method          TEXT NOT NULL,
    notes           TEXT NOT NULL DEFAULT '',
    -- Target of hazard_rating's composite foreign key: a rating can only cite
    -- a source of its own hazard type.
    UNIQUE (id, hazard_type)
);

-- The geographic units ratings are given for. `key` is '<ISO 3166-1 alpha-2
-- country>-<national code of the unit>', for Portugal 'PT-<4-digit DICO code
-- of the municipality>' (for example PT-1106 = Lisboa). Names are not unique
-- ("Lagoa" exists twice in Portugal), so lookups go by key.
CREATE TABLE hazard_region (
    key         TEXT PRIMARY KEY CHECK (key ~ '^[A-Z]{2}-[0-9A-Z]+$'),
    country     TEXT NOT NULL CHECK (country ~ '^[A-Z]{2}$'),
    name        TEXT NOT NULL,
    -- District or island: tells apart regions that share a name.
    parent_name TEXT NOT NULL DEFAULT '',
    -- name, lower-cased, without diacritics, hyphens as spaces: what name
    -- searches compare to. The table has a few hundred rows, so no index.
    name_search TEXT NOT NULL,
    CHECK (key LIKE country || '-%')
);

-- rating: 1 = very low hazard ... 5 = very high hazard (higher is worse; the
-- opposite direction of a listing's Score). A hazard without usable source
-- data for a region has no row at all.
CREATE TABLE hazard_rating (
    region_key  TEXT NOT NULL REFERENCES hazard_region (key) ON DELETE CASCADE,
    hazard_type TEXT NOT NULL,
    rating      SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    source_id   TEXT NOT NULL,
    -- Last time this row's rating or source changed (not bumped by a re-sync
    -- of identical data).
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (region_key, hazard_type),
    FOREIGN KEY (source_id, hazard_type) REFERENCES hazard_source (id, hazard_type)
);
