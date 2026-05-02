-- Per-server opt-in flag for hub-admin RCON delegation. Operator sets
-- it in the collector's per-server config; the collector publishes the
-- value with each heartbeat roster, hub stores it for UI gating. The
-- collector remains the source of truth and re-validates on every
-- proxied RCON request — this column is purely advisory for the UI.
--
-- Run with the trinity service stopped:
--   sudo systemctl stop trinity
--   sudo cp /var/lib/trinity/trinity.db /var/lib/trinity/trinity.db.bak.$(date +%Y%m%d-%H%M%S)
--   sudo -u quake sqlite3 /var/lib/trinity/trinity.db < migrations/2026-05-02-admin-delegation.sql

ALTER TABLE servers ADD COLUMN admin_delegation_enabled INTEGER NOT NULL DEFAULT 0;
