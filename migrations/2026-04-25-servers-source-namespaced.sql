-- Make server identity (source, address) instead of address alone, drop
-- the now-unused log_path and remote_address columns, and require source
-- to be non-empty on every row.
--
-- Run with the trinity service stopped:
--   sudo systemctl stop trinity
--   sudo cp /var/lib/trinity/trinity.db /var/lib/trinity/trinity.db.bak.$(date +%Y%m%d-%H%M%S)
--   sudo -u quake sqlite3 /var/lib/trinity/trinity.db < migrations/2026-04-25-servers-source-namespaced.sql
--   # ...then deploy the new binary and restart.
--
-- The new servers_new table enforces NOT NULL + CHECK(source <> '').
-- If any existing row has a NULL or empty source the INSERT will fail
-- and the whole transaction rolls back; fix the offending rows by hand
-- (set the right source) and re-run.

PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;

CREATE TABLE servers_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    address TEXT NOT NULL,
    last_match_uuid TEXT,
    last_match_ended_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    source TEXT NOT NULL CHECK(source <> ''),
    local_id INTEGER,
    is_remote INTEGER NOT NULL DEFAULT 0,
    last_heartbeat_at TIMESTAMP,
    demo_base_url TEXT,
    source_version TEXT,
    UNIQUE(source, address)
);

INSERT INTO servers_new
  (id, name, address, last_match_uuid, last_match_ended_at, created_at,
   source, local_id, is_remote, last_heartbeat_at, demo_base_url, source_version)
  SELECT id, name, address, last_match_uuid, last_match_ended_at, created_at,
         source, local_id, is_remote, last_heartbeat_at, demo_base_url, source_version
  FROM servers;

DROP TABLE servers;
ALTER TABLE servers_new RENAME TO servers;

CREATE INDEX idx_servers_source_local_id ON servers (source, local_id);

COMMIT;
PRAGMA foreign_keys=ON;
