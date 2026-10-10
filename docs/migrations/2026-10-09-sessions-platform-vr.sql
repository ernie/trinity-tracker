-- 2026-10-09 — Session platform (cl_platform) and VR use.
-- Run on existing installs before deploying the release that adds the columns.
-- Existing sessions keep an empty platform and is_vr false.
ALTER TABLE sessions ADD COLUMN platform TEXT DEFAULT '';
ALTER TABLE sessions ADD COLUMN is_vr BOOLEAN DEFAULT FALSE;
