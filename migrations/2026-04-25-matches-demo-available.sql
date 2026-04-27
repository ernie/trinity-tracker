-- Add matches.demo_available so the hub can decide whether to render a
-- "play demo" button without guessing or HEAD-checking the source.
-- Flipped to 1 by the FactDemoFinalized handler when trinity-engine
-- emits a "DemoSaved:" log line (engine change in trinity-engine
-- sv_tv.c). Existing rows default to 0; backfill local matches via
-- scripts/backfill-demo-available.sh after this migration.
--
-- Run with the trinity service stopped:
--   sudo systemctl stop trinity
--   sudo cp /var/lib/trinity/trinity.db /var/lib/trinity/trinity.db.bak.$(date +%Y%m%d-%H%M%S)
--   sudo -u quake sqlite3 /var/lib/trinity/trinity.db < migrations/2026-04-25-matches-demo-available.sql

ALTER TABLE matches ADD COLUMN demo_available INTEGER NOT NULL DEFAULT 0;
