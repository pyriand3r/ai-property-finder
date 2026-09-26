-- listing_note is edit-in-place (RF.4), not delete-and-recreate — every
-- other editable table already has updated_at, this one was missing it.
ALTER TABLE listing_note ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
