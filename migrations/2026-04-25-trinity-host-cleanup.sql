-- Host-specific cleanup for trinity.ernie.io after the name → key
-- migration. Renames the local source from "main" to "Trinity" and
-- shortens the auto-slugified keys to the human-typed second half
-- of their old display names. Run only on this hub; new deployments
-- don't need this file.
--
-- Run after 2026-04-25-servers-name-to-key.sql, with trinity stopped:
--   sudo -u quake sqlite3 /var/lib/trinity/trinity.db < migrations/2026-04-25-trinity-host-cleanup.sql

BEGIN TRANSACTION;

-- 1. Rename the source. servers.source, sources.source, and
--    source_progress.source all hold "main" and need the same flip.
UPDATE sources         SET source = 'Trinity' WHERE source = 'main';
UPDATE servers         SET source = 'Trinity' WHERE source = 'main';
UPDATE source_progress SET source = 'Trinity' WHERE source = 'main';

-- 2. Shorten the auto-slugified keys ("Trinity---FFA" etc.) to the
--    bare second-half names. CPMA variants get a "-CPMA" suffix.
UPDATE servers SET key = 'FFA'      WHERE source = 'Trinity' AND key = 'Trinity---FFA';
UPDATE servers SET key = '1v1'      WHERE source = 'Trinity' AND key = 'Trinity---1v1';
UPDATE servers SET key = 'TDM'      WHERE source = 'Trinity' AND key = 'Trinity---TDM';
UPDATE servers SET key = 'CTF'      WHERE source = 'Trinity' AND key = 'Trinity---CTF';
UPDATE servers SET key = 'FFA-CPMA' WHERE source = 'Trinity' AND key = 'Trinity---FFA-CPMA';
UPDATE servers SET key = '1v1-CPMA' WHERE source = 'Trinity' AND key = 'Trinity---1v1-CPMA';

COMMIT;
