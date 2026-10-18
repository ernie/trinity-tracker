-- Trinity tracker database schema

-- Servers being monitored
CREATE TABLE IF NOT EXISTS servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    -- Stable, case-insensitive identifier, so the address can change
    -- without splitting history. The UI shows "<source> / <key>".
    key TEXT NOT NULL CHECK(key <> ''),
    address TEXT NOT NULL,
    last_match_uuid TEXT,
    last_match_ended_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    -- Distributed tracking. source names the collector (a logical
    -- reference to sources.source, which also holds is_remote).
    -- Identity is (source, key), so a new address doesn't fork history.
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
    -- 1 once a match_start on this server reported
    -- handshake_required=true. Until then the hub rejects player_join,
    -- player_leave and live events for the server, even from a collector
    -- that skips its own gate. handleMatchStart updates it on every
    -- match_start, accepted or rejected.
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

-- Logical players: one person, who can have several GUIDs
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

-- Player sessions (joins and leaves), per GUID
CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    player_guid_id INTEGER NOT NULL REFERENCES player_guids(id) ON DELETE CASCADE,
    server_id INTEGER NOT NULL REFERENCES servers(id),
    joined_at TIMESTAMP NOT NULL,
    left_at TIMESTAMP,
    duration_seconds INTEGER,
    ip_address TEXT DEFAULT '',
    client_engine TEXT DEFAULT '',
    client_version TEXT DEFAULT '',
    platform TEXT DEFAULT '',          -- cl_platform, from the Trinity handshake
    is_vr BOOLEAN DEFAULT FALSE        -- set once VR runs, never cleared
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
    mode TEXT,
    -- Flipped to 1 by FactDemoFinalized; the UI uses this to decide
    -- whether to render a "play demo" button for the match.
    demo_available INTEGER NOT NULL DEFAULT 0,
    -- Admin-set flag that puts this match in the featured demos pool
    -- behind the landing page's Watch a fight door.
    is_featured INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_matches_server_id ON matches(server_id);
CREATE INDEX IF NOT EXISTS idx_matches_started_at ON matches(started_at);
CREATE INDEX IF NOT EXISTS idx_matches_ended_at ON matches(ended_at);
CREATE INDEX IF NOT EXISTS idx_matches_uuid ON matches(uuid);
CREATE INDEX IF NOT EXISTS idx_matches_has_human_player ON matches(has_human_player);
CREATE INDEX IF NOT EXISTS idx_matches_is_featured ON matches(is_featured) WHERE is_featured = 1;

-- Player stats per match, per GUID that earned them. client_id tells
-- same-named bots in one match apart.
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
    skulls_delivered INTEGER DEFAULT 0,
    obelisks_destroyed INTEGER DEFAULT 0,
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
    game_token TEXT NOT NULL DEFAULT '',
    -- The in-game name pushed to the server on auth_ok, seeded from the
    -- player's in-game name with the VR tag stripped. Empty means no
    -- override, so the QVM uses whatever the client sends. May contain
    -- color codes (^0-^7) and spaces.
    display_name TEXT NOT NULL DEFAULT '',
    -- display_name in its unique form (storage.CanonicalizeDisplayName).
    -- The partial unique index below keeps two accounts from locking
    -- names that look the same in game, such as "^1Foo" and "^7Foo".
    display_name_canonical TEXT NOT NULL DEFAULT '',
    -- NULL = no explicit choice; frontend falls back to 'victories'.
    featured_honor TEXT,
    -- Account-chosen profile icon as "model/skin" (e.g. "sarge/krusade").
    -- NULL = follow the most recent in-game model.
    portrait TEXT,
    -- Non-NULL disables the account on every credential path (web
    -- login, cli-login, game-login, in-game auth, PAT lookup).
    disabled_at TIMESTAMP,
    -- Web-session revocation: JWTs carry this at mint and die when it
    -- no longer matches (bumped on any password-hash write and by
    -- logout-all). PATs are unaffected.
    token_version INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_player_id ON users(player_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_display_name_canonical
    ON users(display_name_canonical)
    WHERE display_name_canonical != '';

-- Personal access tokens: the CLI credential (web sessions use JWTs).
-- Only the SHA-256 of the token is stored; the plaintext is returned
-- once at mint. Revocation ends a token's life; expires_at stays NULL
-- by default.
CREATE TABLE IF NOT EXISTS pat (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT '',
    token_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP,
    revoked_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pat_user_id ON pat(user_id);

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

-- Per-source event watermark. An envelope whose seq is at or below
-- consumed_seq is a redelivery and is dropped. last_consumed_ts is the
-- timestamp seen at that seq, for monitoring. A collector whose own
-- watermark was wiped takes its starting seq from here.
CREATE TABLE IF NOT EXISTS source_progress (
    source           TEXT PRIMARY KEY,
    consumed_seq     INTEGER NOT NULL DEFAULT 0,
    last_consumed_ts TIMESTAMP NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    updated_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Sources known to the hub, one per collector. Ingest refuses anything
-- published for a source without a row here. A combined hub and
-- collector inserts its own row at startup. servers.source references
-- this table by name, enforced in application code.
CREATE TABLE IF NOT EXISTS sources (
    source            TEXT PRIMARY KEY,
    demo_base_url     TEXT NOT NULL DEFAULT '',
    version           TEXT NOT NULL DEFAULT '',
    last_heartbeat_at TIMESTAMP,
    is_remote         INTEGER NOT NULL DEFAULT 1,
    -- NATS user public key of this source's collector, written by
    -- AuthStore.MintUserCreds and empty until creds are minted.
    -- nats-server reports it as AuthorizedUser, so the directory gate
    -- and the hub poller use it (ConnzOptions.User) to find the
    -- source's live connection.
    user_pubkey       TEXT NOT NULL DEFAULT '',
    -- 1 while the source's collector is allowed to publish. Mirrors
    -- status='active'; the ingest gate (SourceRegistry) keys off this.
    active            INTEGER NOT NULL DEFAULT 1,
    -- Ownership and lifecycle. owner_user_id is NULL only for the hub's
    -- own local source. status:
    --   pending:  awaiting admin approval, no creds yet
    --   active:   approved, creds minted (active=1)
    --   rejected: admin declined; rejection_reason says why
    --   left:     the owner left; the row stays and the owner can rejoin
    --   revoked:  admin disabled it; only an admin can reactivate it
    --
    -- An owner can hold several active sources, one per host, each with
    -- its own creds and rotate and leave controls, but only one pending
    -- request at a time.
    owner_user_id     INTEGER REFERENCES users(id),
    status            TEXT NOT NULL DEFAULT 'active'
                          CHECK(status IN ('pending', 'active', 'rejected', 'left', 'revoked')),
    -- requested_purpose is the requester's "What is this for?" text,
    -- shown to the admin who reviews the request.
    requested_purpose TEXT,
    rejection_reason  TEXT,
    status_changed_at TIMESTAMP,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sources_owner_user_id ON sources(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_sources_status ON sources(status) WHERE status != 'active';

-- Audit trail for source lifecycle actions and RCON use (see
-- WriteSourceAudit).
CREATE TABLE IF NOT EXISTS source_audit (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    source          TEXT NOT NULL REFERENCES sources(source),
    actor_user_id   INTEGER REFERENCES users(id),  -- NULL for system actions
    action          TEXT NOT NULL,
    detail          TEXT,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_source_audit_source ON source_audit(source, created_at);

-- Snapshot of the Q3 directory's validated registry, written at
-- graceful shutdown and restored at startup when it's recent enough
-- to be a routine restart (tracker.hub.directory.persisted_freshness).
-- server_id is advisory; the gate decides membership.
CREATE TABLE IF NOT EXISTS directory_registrations (
    addr          TEXT PRIMARY KEY,
    server_id     INTEGER NOT NULL,
    protocol      INTEGER NOT NULL,
    gamename      TEXT NOT NULL,
    engine        TEXT NOT NULL,
    clients       INTEGER NOT NULL,
    max_clients   INTEGER NOT NULL,
    gametype      INTEGER NOT NULL,
    validated_at  INTEGER NOT NULL,
    expires_at    INTEGER NOT NULL
);
