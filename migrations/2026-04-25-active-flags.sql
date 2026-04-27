-- Add the `active` flag to both sources and servers. Defaults to 1 so
-- existing rows stay live. DeactivateSource (renamed from DeleteSource)
-- now flips both flags via cascade instead of DELETEing rows; old
-- matches keep their server_id reference and the UI dims inactive
-- rows rather than dropping them silently.
--
-- Run with the trinity service stopped:
--   sudo systemctl stop trinity
--   sudo cp /var/lib/trinity/trinity.db /var/lib/trinity/trinity.db.bak.$(date +%Y%m%d-%H%M%S)
--   sudo -u quake sqlite3 /var/lib/trinity/trinity.db < migrations/2026-04-25-active-flags.sql

ALTER TABLE sources ADD COLUMN active INTEGER NOT NULL DEFAULT 1;
ALTER TABLE servers ADD COLUMN active INTEGER NOT NULL DEFAULT 1;
