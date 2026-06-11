-- 2026-06-11 — Account-chosen profile icon ("model/skin").
-- Run on existing installs before deploying the release that adds the column.
ALTER TABLE users ADD COLUMN portrait TEXT;
