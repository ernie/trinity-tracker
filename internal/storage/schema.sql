-- Q3A Stats Database Schema

-- Servers being monitored
CREATE TABLE IF NOT EXISTS servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    -- Stable, case-insensitive identifier. Same character set as
    -- sources.source: alnum/underscore/hyphen, 1-64 chars. Address
    -- can change without splitting history. Display in the UI is
    -- "<source> / <key>" (case preserved).
    key TEXT NOT NULL CHECK(key <> ''),
    address TEXT NOT NULL,
    last_match_uuid TEXT,
    last_match_ended_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    -- Distributed-tracking fields. source is the admin-chosen collector
    -- identifier (FK-ish to sources.source). Whether the source is
    -- remote lives on the sources row; readers JOIN sources for
    -- sources.is_remote rather than duplicating it here. Identity is
    -- (source, key): renaming or rerouting (address change) doesn't
    -- fork history.
    source TEXT NOT NULL CHECK(source <> ''),
    local_id INTEGER,
    last_heartbeat_at TIMESTAMP,
    demo_base_url TEXT,
    source_version TEXT,
    -- 1 while the server is configured to run on its source. Flipped
    -- to 0 by DeactivateSource (cascade) or by collector startup when
    -- a server is removed from cfg.Q3Servers. UI dims inactive rows;
    -- the live cards section filters them out.
    active INTEGER NOT NULL DEFAULT 1,
    -- 1 once we've observed a match_start with handshake_required=true
    -- on this server. Until then the hub rejects player_join /
    -- player_leave / live events for the server (so a collector that
    -- somehow publishes despite the source-side gate still can't
    -- pollute the hub). Updated by handleMatchStart on every observed
    -- match_start, both accept and reject paths.
    handshake_required INTEGER NOT NULL DEFAULT 0,
    -- Per-server opt-in by the source operator: when 1, a hub admin
    -- (is_admin=1) is allowed to RCON this server even if they don't
    -- own its source. Mirrored on every collector heartbeat from the
    -- collector's per-server config; the collector remains authoritative
    -- and refuses proxy requests not matching its current state. Owners
    -- of the source can RCON regardless of this flag.
    admin_delegation_enabled INTEGER NOT NULL DEFAULT 0,
    UNIQUE(source, key COLLATE NOCASE)
);

CREATE INDEX IF NOT EXISTS idx_servers_source_local_id ON servers (source, local_id);

-- Logical players (the "person" - can have multiple GUIDs)
CREATE TABLE IF NOT EXISTS players (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,              -- display name (from most recent GUID activity)
    clean_name TEXT NOT NULL,
    first_seen TIMESTAMP,
    last_seen TIMESTAMP,
    total_playtime_seconds INTEGER DEFAULT 0,
    is_bot BOOLEAN DEFAULT FALSE,
    is_vr BOOLEAN DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_players_clean_name ON players(clean_name);
CREATE INDEX IF NOT EXISTS idx_players_is_bot ON players(is_bot);
CREATE INDEX IF NOT EXISTS idx_players_last_seen ON players(last_seen);

-- GUIDs belonging to players (one GUID = one client identity)
CREATE TABLE IF NOT EXISTS player_guids (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    guid TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,              -- name when using this GUID
    clean_name TEXT NOT NULL,
    first_seen TIMESTAMP,
    last_seen TIMESTAMP,
    is_bot BOOLEAN DEFAULT FALSE,
    is_vr BOOLEAN DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_player_guids_player_id ON player_guids(player_id);
CREATE INDEX IF NOT EXISTS idx_player_guids_guid ON player_guids(guid);
CREATE INDEX IF NOT EXISTS idx_player_guids_clean_name ON player_guids(clean_name);

-- Historical player names per GUID (for "also known as" display)
CREATE TABLE IF NOT EXISTS player_names (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    player_guid_id INTEGER NOT NULL REFERENCES player_guids(id) ON DELETE CASCADE,
    name TEXT NOT NULL,              -- original color-coded name
    clean_name TEXT NOT NULL,        -- stripped name
    first_seen TIMESTAMP NOT NULL,
    last_seen TIMESTAMP NOT NULL,
    UNIQUE(player_guid_id, clean_name)
);

CREATE INDEX IF NOT EXISTS idx_player_names_player_guid_id ON player_names(player_guid_id);

-- Player sessions (joins/leaves) - linked to specific GUID
CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    player_guid_id INTEGER NOT NULL REFERENCES player_guids(id) ON DELETE CASCADE,
    server_id INTEGER NOT NULL REFERENCES servers(id),
    joined_at TIMESTAMP NOT NULL,
    left_at TIMESTAMP,
    duration_seconds INTEGER,
    ip_address TEXT DEFAULT '',
    client_engine TEXT DEFAULT '',
    client_version TEXT DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_sessions_player_guid_id ON sessions(player_guid_id);
CREATE INDEX IF NOT EXISTS idx_sessions_server_id ON sessions(server_id);
CREATE INDEX IF NOT EXISTS idx_sessions_joined_at ON sessions(joined_at);

-- Match/game records
CREATE TABLE IF NOT EXISTS matches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,      -- unique match identifier from game
    server_id INTEGER NOT NULL REFERENCES servers(id),
    map_name TEXT,
    game_type TEXT,
    started_at TIMESTAMP,
    ended_at TIMESTAMP,
    exit_reason TEXT,
    red_score INTEGER,
    blue_score INTEGER,
    has_human_player BOOLEAN DEFAULT FALSE,
    movement TEXT,
    gameplay TEXT,
    -- Flipped to 1 by FactDemoFinalized; the UI uses this to decide
    -- whether to render a "play demo" button for the match.
    demo_available INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_matches_server_id ON matches(server_id);
CREATE INDEX IF NOT EXISTS idx_matches_started_at ON matches(started_at);
CREATE INDEX IF NOT EXISTS idx_matches_ended_at ON matches(ended_at);
CREATE INDEX IF NOT EXISTS idx_matches_uuid ON matches(uuid);
CREATE INDEX IF NOT EXISTS idx_matches_has_human_player ON matches(has_human_player);

-- Player stats per match - linked to specific GUID that earned them
-- client_id allows tracking duplicate bots (same name) in the same match
CREATE TABLE IF NOT EXISTS match_player_stats (
    match_id INTEGER NOT NULL REFERENCES matches(id),
    player_guid_id INTEGER NOT NULL REFERENCES player_guids(id) ON DELETE CASCADE,
    client_id INTEGER NOT NULL DEFAULT 0,
    frags INTEGER DEFAULT 0,
    deaths INTEGER DEFAULT 0,
    joined_late BOOLEAN DEFAULT FALSE,
    completed BOOLEAN DEFAULT FALSE,
    joined_at TIMESTAMP,
    captures INTEGER DEFAULT 0,
    flag_returns INTEGER DEFAULT 0,
    assists INTEGER DEFAULT 0,
    impressives INTEGER DEFAULT 0,
    excellents INTEGER DEFAULT 0,
    humiliations INTEGER DEFAULT 0,
    defends INTEGER DEFAULT 0,
    victories INTEGER DEFAULT 0,
    score INTEGER,
    team INTEGER,
    model TEXT,
    skill REAL,
    is_vr BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (match_id, player_guid_id, client_id)
);

CREATE INDEX IF NOT EXISTS idx_match_player_stats_player_guid_id ON match_player_stats(player_guid_id);
CREATE INDEX IF NOT EXISTS idx_match_player_stats_completed ON match_player_stats(completed);
CREATE INDEX IF NOT EXISTS idx_match_player_stats_covering ON match_player_stats(player_guid_id, match_id, frags, deaths);

-- Users for authentication
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_admin BOOLEAN DEFAULT FALSE,
    player_id INTEGER UNIQUE REFERENCES players(id) ON DELETE SET NULL,
    password_change_required BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_login TIMESTAMP,
    game_token TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_player_id ON users(player_id);

-- Link codes for account linking via game chat
-- user_id is nullable: NULL = claim code (player-initiated), NOT NULL = link code (user-initiated)
CREATE TABLE IF NOT EXISTS link_codes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT NOT NULL UNIQUE,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    player_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    used_at TIMESTAMP,
    used_by_guid TEXT
);

CREATE INDEX IF NOT EXISTS idx_link_codes_code ON link_codes(code);
CREATE INDEX IF NOT EXISTS idx_link_codes_expires_at ON link_codes(expires_at);

-- Distributed tracking: hub bookkeeping for sources/collectors.

-- Per-source event watermark. Tuple of (last_consumed_ts, consumed_seq)
-- defines forward progress: a re-published event is identified by a
-- timestamp older than last_consumed_ts (history; drop), the same
-- timestamp with seq <= consumed_seq (JetStream redelivery; drop), or
-- something newer (accept and advance both fields). Time-anchored
-- because seq alone resets to 1 on a fresh-install collector and would
-- otherwise mass-drop the new collector's events against a stale
-- consumed_seq from the prior instance.
CREATE TABLE IF NOT EXISTS source_progress (
    source           TEXT PRIMARY KEY,
    consumed_seq     INTEGER NOT NULL DEFAULT 0,
    last_consumed_ts TIMESTAMP NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    updated_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Sources known to the hub. Admin pre-provisions every collector here
-- (hub mints NATS creds at the same moment); a collector publishing
-- anything for a source not in this table is refused at ingest. Local
-- hub+collector installs insert a row for themselves at startup.
-- servers.source references this table logically; enforcement is
-- application-level (SQLite FK enforcement is opt-in per connection).
CREATE TABLE IF NOT EXISTS sources (
    source            TEXT PRIMARY KEY,
    demo_base_url     TEXT NOT NULL DEFAULT '',
    version           TEXT NOT NULL DEFAULT '',
    last_heartbeat_at TIMESTAMP,
    is_remote         INTEGER NOT NULL DEFAULT 1,
    -- NATS user NKey public key for this source's authenticated
    -- collector connection. Written by AuthStore.MintUserCreds, read
    -- by the directory gate and the hub poller (joined into their
    -- pollable-server queries) to identify which live NATS connection
    -- belongs to this source. Empty until the operator approves and
    -- creds are minted. The user pubkey is what nats-server reports
    -- as AuthorizedUser for JWT-authenticated clients, so it's the
    -- right key for ConnzOptions.User filtering.
    user_pubkey       TEXT NOT NULL DEFAULT '',
    -- 1 while the source's collector is allowed to publish. Mirrors
    -- status='active'; the ingest gate (SourceRegistry) keys off this.
    active            INTEGER NOT NULL DEFAULT 1,
    -- Self-service ownership and lifecycle. owner_user_id NULL means
    -- admin-minted (legacy or hub-internal); status drives the
    -- user-facing flow:
    --   pending  -- awaiting admin approval, no creds yet
    --   active   -- approved + creds minted (active=1)
    --   rejected -- admin declined; rejection_reason explains why
    --   left     -- owner self-departed; row preserved, can self-rejoin
    --   revoked  -- admin punitive disable; only admin can re-enable
    --
    -- One owner can hold multiple active sources at once — the
    -- recommended deployment is one source per physical host or
    -- location, so geographic distribution gets per-collector creds
    -- and per-collector rotate/leave controls. Only one *pending*
    -- request per owner at a time, to keep the admin queue tidy.
    owner_user_id     INTEGER REFERENCES users(id),
    status            TEXT NOT NULL DEFAULT 'active'
                          CHECK(status IN ('pending', 'active', 'rejected', 'left', 'revoked')),
    -- requested_purpose is the free-text "what is this for?" that
    -- helps admins triage. The collector's reachable URL arrives via
    -- registration heartbeats (demo_base_url above); no need to ask
    -- the operator for it up front.
    requested_purpose TEXT,
    rejection_reason  TEXT,
    status_changed_at TIMESTAMP,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sources_owner_user_id ON sources(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_sources_status ON sources(status) WHERE status != 'active';

-- Audit trail for source lifecycle actions (request/approve/reject/
-- rotate/leave/rejoin/revoke). Sized for eventual RCON-via-hub use:
-- the same table will record rcon.kick / rcon.exec calls.
CREATE TABLE IF NOT EXISTS source_audit (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    source          TEXT NOT NULL REFERENCES sources(source),
    actor_user_id   INTEGER REFERENCES users(id),  -- NULL for system actions
    action          TEXT NOT NULL,
    detail          TEXT,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_source_audit_source ON source_audit(source, created_at);
