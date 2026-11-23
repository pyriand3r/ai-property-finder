-- A criterion is either 'proposed' (suggested by the Agent, not yet
-- approved: ignored by evaluation and scoring) or 'active' (docs/adr/0024).
-- DEFAULT 'active' sets every existing row to active in the same statement,
-- so no profile loses its criteria; the plain form keeps inserting 'active'.
ALTER TABLE criterion
    ADD COLUMN status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('proposed', 'active'));
