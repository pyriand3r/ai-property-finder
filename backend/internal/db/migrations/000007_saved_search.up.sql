-- Saved portal search-result URLs that the app searches on a schedule
-- (docs/adr/0029, spec 01-17, issue #19).
--
-- A run is a portal.search task (docs/adr/0028); this table only keeps the
-- user's saved request, the schedule and the outcome of the last run.
-- status is TEXT + CHECK like criterion.status (migration 000004).
CREATE TABLE saved_search (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id     UUID        NOT NULL REFERENCES search_profile(id) ON DELETE CASCADE,
    name           TEXT        NOT NULL,
    -- The request of the search_portal Tool (spec 01-16).
    url            TEXT        NOT NULL,
    page_param     TEXT        NOT NULL DEFAULT '',
    max_pages      INTEGER     NOT NULL DEFAULT 1 CHECK (max_pages BETWEEN 1 AND 5),
    link_contains  TEXT        NOT NULL DEFAULT '',
    -- Schedule: every interval_hours hours. next_run_at is NULL while disabled.
    interval_hours INTEGER     NOT NULL DEFAULT 24 CHECK (interval_hours BETWEEN 1 AND 720),
    enabled        BOOLEAN     NOT NULL DEFAULT TRUE,
    next_run_at    TIMESTAMPTZ,
    -- Whether a run hands new links to the Agent in the profile's active
    -- chat session, and at most how many per run.
    import_new     BOOLEAN     NOT NULL DEFAULT TRUE,
    max_import     INTEGER     NOT NULL DEFAULT 3 CHECK (max_import BETWEEN 1 AND 10),
    -- matchKey of every link already reported (or handed to the Agent) by an
    -- earlier run, so a run only reports links that are new since then.
    seen_links     JSONB       NOT NULL DEFAULT '[]'::jsonb,
    -- Outcome of the last run. 'running' is the claim that keeps a run from
    -- starting twice; Recover at boot turns a stale one into 'failed'.
    last_status    TEXT        NOT NULL DEFAULT 'idle' CHECK (last_status IN ('idle', 'running', 'succeeded', 'failed')),
    last_run_at    TIMESTAMPTZ,
    last_task_id   UUID,
    last_found     INTEGER     NOT NULL DEFAULT 0,
    last_new       INTEGER     NOT NULL DEFAULT 0,
    last_imported  INTEGER     NOT NULL DEFAULT 0,
    last_summary   TEXT        NOT NULL DEFAULT '',
    last_error     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (profile_id, name)
);
CREATE INDEX idx_saved_search_due ON saved_search (next_run_at) WHERE enabled;
