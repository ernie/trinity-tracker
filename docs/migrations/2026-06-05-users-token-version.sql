-- 2026-06-05 — Web-session token versioning (revocable JWTs)
-- Run on existing installs before deploying the release that adds the
-- column; the new binary's user SELECTs reference it at startup.
-- All outstanding web JWTs predate versioning (token_ver 0 ≠ 1) and
-- require one re-login after deploy. PATs are unaffected.
ALTER TABLE users ADD COLUMN token_version INTEGER NOT NULL DEFAULT 1;
