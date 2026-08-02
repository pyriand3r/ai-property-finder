-- Reverse of 000001_init.up.sql. Forward-only migrations in production mean
-- this file exists only to support local and integration-test teardowns.

DROP TABLE IF EXISTS context_revision;
DROP TABLE IF EXISTS chat_message;
DROP TABLE IF EXISTS chat_session;
DROP TABLE IF EXISTS context;

DROP TABLE IF EXISTS task;
DROP TABLE IF EXISTS app_config;
DROP TABLE IF EXISTS listing_note;
DROP TABLE IF EXISTS listing_criterion_grade;

ALTER TABLE listing DROP COLUMN IF EXISTS title_image_id;
DROP TABLE IF EXISTS listing_image;
DROP TABLE IF EXISTS listing;
DROP TABLE IF EXISTS criterion;
DROP TABLE IF EXISTS search_profile;

DROP TYPE IF EXISTS source_type;
DROP TYPE IF EXISTS context_scope;
DROP TYPE IF EXISTS chat_role;
DROP TYPE IF EXISTS criterion_grade;
DROP TYPE IF EXISTS listing_status;
