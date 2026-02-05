-- Q3A Stats Database Schema

-- Servers being monitored
CREATE TABLE IF NOT EXISTS servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    address TEXT NOT NULL UNIQUE,
    log_path TEXT,
    last_match_uuid TEXT,
    last_match_ended_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

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
    ip_address TEXT DEFAULT ''
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
    has_human_player BOOLEAN DEFAULT FALSE
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
    last_login TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_player_id ON users(player_id);

-- Link codes for account linking via game chat
CREATE TABLE IF NOT EXISTS link_codes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT NOT NULL UNIQUE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    player_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    used_at TIMESTAMP,
    used_by_guid TEXT
);

CREATE INDEX IF NOT EXISTS idx_link_codes_code ON link_codes(code);
CREATE INDEX IF NOT EXISTS idx_link_codes_expires_at ON link_codes(expires_at);
