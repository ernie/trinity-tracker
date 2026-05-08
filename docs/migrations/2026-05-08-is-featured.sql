-- 2026-05-08 — Featured demos pool (landing page)
-- Run on existing installs before deploying the release that adds the column.
ALTER TABLE matches ADD COLUMN is_featured INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_matches_is_featured ON matches(is_featured) WHERE is_featured = 1;
