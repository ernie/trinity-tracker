-- 2026-05-23 — Account-canonical display name with per-account uniqueness
-- Run on existing installs before deploying the release that adds the columns.
-- Existing users' display_name is populated lazily on their next auth.
ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN display_name_canonical TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_display_name_canonical
    ON users(display_name_canonical)
    WHERE display_name_canonical != '';
