-- Hazard data sources suggested by the Agent or an MCP client, waiting for the
-- user's review (docs/adr/0027, issue #46).
--
-- A suggestion is NOT a hazard_source: it holds no ratings and feeds nothing.
-- 'approved' means the user cleared the source for ingestion, which is a
-- release step (a new dataset file, docs/adr/0025). 'rejected' rows stay so
-- the same source is not suggested again.
--
-- status is TEXT + CHECK like criterion.status (migration 000004).
CREATE TABLE hazard_source_suggestion (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    country     TEXT NOT NULL CHECK (country ~ '^[A-Z]{2}$'),
    hazard_type TEXT NOT NULL CHECK (hazard_type IN ('wildfire', 'flood', 'earthquake', 'landslide')),
    -- Part of the country the source covers; '' = all of it.
    scope       TEXT NOT NULL DEFAULT '',
    title       TEXT NOT NULL,
    publisher   TEXT NOT NULL,
    url         TEXT NOT NULL,
    -- url without scheme, 'www.', fragment and trailing slash, lower-cased
    -- host: the identity of a source (hazard.SourceURLKey).
    url_key     TEXT NOT NULL,
    license     TEXT NOT NULL DEFAULT '',
    -- Why the Agent thinks this source is useful and what it covers.
    rationale   TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed', 'approved', 'rejected')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_at  TIMESTAMPTZ,
    UNIQUE (country, hazard_type, url_key)
);
