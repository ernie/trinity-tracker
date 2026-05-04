-- Persist the directory server's validated registry across hub
-- restarts. The hub writes the table once at graceful shutdown and
-- restores from it at startup, only if the snapshot is fresh enough
-- to plausibly be a routine restart/deploy (controlled by
-- tracker.hub.directory.persisted_freshness, default 5m).
--
-- Run with the trinity service stopped:
--   sudo systemctl stop trinity
--   sudo cp /var/lib/trinity/trinity.db /var/lib/trinity/trinity.db.bak.$(date +%Y%m%d-%H%M%S)
--   sudo -u quake sqlite3 /var/lib/trinity/trinity.db < migrations/2026-05-04-directory-registrations.sql

CREATE TABLE IF NOT EXISTS directory_registrations (
    addr          TEXT PRIMARY KEY,    -- netip.AddrPort.String(), e.g. "1.2.3.4:27960"
    server_id     INTEGER NOT NULL,    -- servers.id at the time of validation, advisory only
    protocol      INTEGER NOT NULL,
    gamename      TEXT NOT NULL,
    engine        TEXT NOT NULL,
    clients       INTEGER NOT NULL,
    max_clients   INTEGER NOT NULL,
    gametype      INTEGER NOT NULL,
    validated_at  INTEGER NOT NULL,    -- unix seconds
    expires_at    INTEGER NOT NULL     -- unix seconds
);
