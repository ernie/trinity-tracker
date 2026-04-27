-- Replace servers.name with servers.key, identity becomes (source, key)
-- with case-insensitive dedup. Display strings are composed at render
-- time as "<source> / <key>" so renames don't fork history.
--
-- This migration auto-slugifies existing names with a simple rule:
-- spaces → '-', then strip '(' and ')'. That handles "Trinity - FFA"
-- → "Trinity---FFA" and "Trinity - FFA (CPMA)" → "Trinity---FFA-CPMA"
-- — ugly but valid. Follow it with the host-specific cleanup file
-- (2026-04-25-trinity-host-cleanup.sql) to set short, presentable
-- keys and rename the local source. New deployments don't need that
-- second file; their cfg already uses clean keys.
--
-- Run with the trinity service stopped:
--   sudo systemctl stop trinity
--   sudo cp /var/lib/trinity/trinity.db /var/lib/trinity/trinity.db.bak.$(date +%Y%m%d-%H%M%S)
--   sudo -u quake sqlite3 /var/lib/trinity/trinity.db < migrations/2026-04-25-servers-name-to-key.sql

PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;

CREATE TABLE servers_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT NOT NULL CHECK(key <> ''),
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
    UNIQUE(source, key COLLATE NOCASE)
);

INSERT INTO servers_new
  (id, key, address, last_match_uuid, last_match_ended_at, created_at,
   source, local_id, is_remote, last_heartbeat_at, demo_base_url, source_version)
  SELECT id,
         -- Naive slugify: strip parens, replace spaces with '-'.
         REPLACE(REPLACE(REPLACE(name, '(', ''), ')', ''), ' ', '-'),
         address, last_match_uuid, last_match_ended_at, created_at,
         source, local_id, is_remote, last_heartbeat_at, demo_base_url, source_version
  FROM servers;

DROP TABLE servers;
ALTER TABLE servers_new RENAME TO servers;

CREATE INDEX idx_servers_source_local_id ON servers (source, local_id);

COMMIT;
PRAGMA foreign_keys=ON;
