package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ernie/trinity-tools/internal/domain"
	_ "modernc.org/sqlite"
)

// formatTimestamp converts time.Time to SQLite-compatible UTC ISO8601 string
// The Z suffix ensures the Go sqlite driver parses it back as UTC
func formatTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

//go:embed schema.sql
var schema string

// Store provides database access
type Store struct {
	db *sql.DB
}

// New creates a new Store with the given database path
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// SQLite only supports one writer at a time, so limit connections
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Enable foreign keys, WAL mode for better performance, and busy timeout for concurrency
	if _, err := db.Exec("PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting pragmas: %w", err)
	}

	// Create tables
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// --- Server methods ---

// UpsertServer creates or updates a server
func (s *Store) UpsertServer(ctx context.Context, srv *domain.Server) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO servers (name, address, log_path)
		VALUES (?, ?, ?)
		ON CONFLICT(address) DO UPDATE SET
			name = excluded.name,
			log_path = excluded.log_path
	`, srv.Name, srv.Address, srv.LogPath)
	if err != nil {
		return err
	}

	// Always query for the ID (LastInsertId unreliable with ON CONFLICT)
	return s.db.QueryRowContext(ctx, "SELECT id FROM servers WHERE address = ?", srv.Address).Scan(&srv.ID)
}

// GetServers returns all servers
func (s *Store) GetServers(ctx context.Context) ([]domain.Server, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, address, log_path, last_match_uuid, last_match_ended_at, created_at FROM servers ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []domain.Server
	for rows.Next() {
		var srv domain.Server
		var logPath, lastMatchUUID sql.NullString
		var lastMatchEndedAt sql.NullTime
		if err := rows.Scan(&srv.ID, &srv.Name, &srv.Address, &logPath, &lastMatchUUID, &lastMatchEndedAt, &srv.CreatedAt); err != nil {
			return nil, err
		}
		srv.LogPath = logPath.String
		if lastMatchUUID.Valid {
			srv.LastMatchUUID = &lastMatchUUID.String
		}
		if lastMatchEndedAt.Valid {
			srv.LastMatchEndedAt = &lastMatchEndedAt.Time
		}
		servers = append(servers, srv)
	}
	return servers, rows.Err()
}

// GetServerByID returns a server by ID
func (s *Store) GetServerByID(ctx context.Context, id int64) (*domain.Server, error) {
	var srv domain.Server
	var logPath, lastMatchUUID sql.NullString
	var lastMatchEndedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, address, log_path, last_match_uuid, last_match_ended_at, created_at FROM servers WHERE id = ?
	`, id).Scan(&srv.ID, &srv.Name, &srv.Address, &logPath, &lastMatchUUID, &lastMatchEndedAt, &srv.CreatedAt)
	if err != nil {
		return nil, err
	}
	srv.LogPath = logPath.String
	if lastMatchUUID.Valid {
		srv.LastMatchUUID = &lastMatchUUID.String
	}
	if lastMatchEndedAt.Valid {
		srv.LastMatchEndedAt = &lastMatchEndedAt.Time
	}
	return &srv, nil
}

// --- Player GUID methods ---

// UpsertPlayerGUID creates or updates a player GUID, creating a new player if needed
// Returns the PlayerGUID with ID and PlayerID populated
func (s *Store) UpsertPlayerGUID(ctx context.Context, guid, name, cleanName string, timestamp time.Time, isVR bool) (*domain.PlayerGUID, error) {
	now := timestamp
	if now.IsZero() {
		now = time.Now().UTC()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	var pg domain.PlayerGUID
	err = tx.QueryRowContext(ctx, `
		SELECT id, player_id, guid, name, clean_name, first_seen, last_seen
		FROM player_guids WHERE guid = ?
	`, guid).Scan(&pg.ID, &pg.PlayerID, &pg.GUID, &pg.Name, &pg.CleanName, &pg.FirstSeen, &pg.LastSeen)

	if err == sql.ErrNoRows {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO players (name, clean_name, first_seen, last_seen, is_vr)
			VALUES (?, ?, ?, ?, ?)
		`, name, cleanName, formatTimestamp(now), formatTimestamp(now), isVR)
		if err != nil {
			return nil, fmt.Errorf("creating player: %w", err)
		}
		playerID, _ := result.LastInsertId()

		result, err = tx.ExecContext(ctx, `
			INSERT INTO player_guids (player_id, guid, name, clean_name, first_seen, last_seen, is_vr)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, playerID, guid, name, cleanName, formatTimestamp(now), formatTimestamp(now), isVR)
		if err != nil {
			return nil, fmt.Errorf("creating player_guid: %w", err)
		}
		pgID, _ := result.LastInsertId()

		pg = domain.PlayerGUID{
			ID:        pgID,
			PlayerID:  playerID,
			GUID:      guid,
			Name:      name,
			CleanName: cleanName,
			FirstSeen: now,
			LastSeen:  now,
			IsVR:      isVR,
		}
	} else if err != nil {
		return nil, err
	} else {
		// Sticky VR: once set to true, never reset to false
		_, err = tx.ExecContext(ctx, `
			UPDATE player_guids SET name = ?, clean_name = ?, last_seen = ?, is_vr = is_vr OR ?
			WHERE id = ?
		`, name, cleanName, formatTimestamp(now), isVR, pg.ID)
		if err != nil {
			return nil, err
		}
		pg.Name = name
		pg.CleanName = cleanName
		pg.LastSeen = now

		_, err = tx.ExecContext(ctx, `
			UPDATE players SET name = ?, clean_name = ?, last_seen = ?, is_vr = is_vr OR ?
			WHERE id = ?
		`, name, cleanName, formatTimestamp(now), isVR, pg.PlayerID)
		if err != nil {
			return nil, err
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO player_names (player_guid_id, name, clean_name, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(player_guid_id, clean_name) DO UPDATE SET
			name = excluded.name,
			last_seen = excluded.last_seen
	`, pg.ID, name, cleanName, formatTimestamp(now), formatTimestamp(now))
	if err != nil {
		return nil, fmt.Errorf("recording player name: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return &pg, nil
}

// UpsertBotPlayerGUID creates or updates a bot player GUID using synthetic GUID format "BOT:cleanname"
// Bots with the same clean name globally share the same player entry
func (s *Store) UpsertBotPlayerGUID(ctx context.Context, name, cleanName string, timestamp time.Time) (*domain.PlayerGUID, error) {
	guid := "BOT:" + cleanName
	now := timestamp
	if now.IsZero() {
		now = time.Now().UTC()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	var pg domain.PlayerGUID
	err = tx.QueryRowContext(ctx, `
		SELECT id, player_id, guid, name, clean_name, first_seen, last_seen, is_bot
		FROM player_guids WHERE guid = ?
	`, guid).Scan(&pg.ID, &pg.PlayerID, &pg.GUID, &pg.Name, &pg.CleanName, &pg.FirstSeen, &pg.LastSeen, &pg.IsBot)

	if err == sql.ErrNoRows {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO players (name, clean_name, first_seen, last_seen, is_bot)
			VALUES (?, ?, ?, ?, TRUE)
		`, name, cleanName, formatTimestamp(now), formatTimestamp(now))
		if err != nil {
			return nil, fmt.Errorf("creating bot player: %w", err)
		}
		playerID, _ := result.LastInsertId()

		result, err = tx.ExecContext(ctx, `
			INSERT INTO player_guids (player_id, guid, name, clean_name, first_seen, last_seen, is_bot)
			VALUES (?, ?, ?, ?, ?, ?, TRUE)
		`, playerID, guid, name, cleanName, formatTimestamp(now), formatTimestamp(now))
		if err != nil {
			return nil, fmt.Errorf("creating bot player_guid: %w", err)
		}
		pgID, _ := result.LastInsertId()

		pg = domain.PlayerGUID{
			ID:        pgID,
			PlayerID:  playerID,
			GUID:      guid,
			Name:      name,
			CleanName: cleanName,
			FirstSeen: now,
			LastSeen:  now,
			IsBot:     true,
		}
	} else if err != nil {
		return nil, err
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE player_guids SET name = ?, clean_name = ?, last_seen = ?
			WHERE id = ?
		`, name, cleanName, formatTimestamp(now), pg.ID)
		if err != nil {
			return nil, err
		}
		pg.Name = name
		pg.CleanName = cleanName
		pg.LastSeen = now

		_, err = tx.ExecContext(ctx, `
			UPDATE players SET name = ?, clean_name = ?, last_seen = ?
			WHERE id = ?
		`, name, cleanName, formatTimestamp(now), pg.PlayerID)
		if err != nil {
			return nil, err
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO player_names (player_guid_id, name, clean_name, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(player_guid_id, clean_name) DO UPDATE SET
			name = excluded.name,
			last_seen = excluded.last_seen
	`, pg.ID, name, cleanName, formatTimestamp(now), formatTimestamp(now))
	if err != nil {
		return nil, fmt.Errorf("recording player name: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return &pg, nil
}

// GetPlayerGUIDByGUID finds a player_guid by GUID string
func (s *Store) GetPlayerGUIDByGUID(ctx context.Context, guid string) (*domain.PlayerGUID, error) {
	var pg domain.PlayerGUID
	err := s.db.QueryRowContext(ctx, `
		SELECT id, player_id, guid, name, clean_name, first_seen, last_seen
		FROM player_guids WHERE guid = ?
	`, guid).Scan(&pg.ID, &pg.PlayerID, &pg.GUID, &pg.Name, &pg.CleanName, &pg.FirstSeen, &pg.LastSeen)
	if err != nil {
		return nil, err
	}
	return &pg, nil
}

// GetPlayerGUIDs returns all GUIDs for a player
func (s *Store) GetPlayerGUIDs(ctx context.Context, playerID int64) ([]domain.PlayerGUID, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, player_id, guid, name, clean_name, first_seen, last_seen, is_vr
		FROM player_guids WHERE player_id = ?
		ORDER BY last_seen DESC
	`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var guids []domain.PlayerGUID
	for rows.Next() {
		var pg domain.PlayerGUID
		if err := rows.Scan(&pg.ID, &pg.PlayerID, &pg.GUID, &pg.Name, &pg.CleanName, &pg.FirstSeen, &pg.LastSeen, &pg.IsVR); err != nil {
			return nil, err
		}
		guids = append(guids, pg)
	}
	return guids, rows.Err()
}

// --- Player methods ---

// GetPlayerByID finds a player by their ID (includes GUIDs)
func (s *Store) GetPlayerByID(ctx context.Context, id int64) (*domain.Player, error) {
	var p domain.Player
	err := s.db.QueryRowContext(ctx, `
		SELECT
			p.id, p.name, p.clean_name, p.first_seen, p.last_seen,
			COALESCE((
				SELECT SUM(s.duration_seconds)
				FROM sessions s
				JOIN player_guids pg ON s.player_guid_id = pg.id
				WHERE pg.player_id = p.id AND s.left_at IS NOT NULL
			), 0) as total_playtime_seconds,
			p.is_bot, p.is_vr
		FROM players p WHERE p.id = ?
	`, id).Scan(&p.ID, &p.Name, &p.CleanName, &p.FirstSeen, &p.LastSeen, &p.TotalPlaytimeSeconds, &p.IsBot, &p.IsVR)
	if err != nil {
		return nil, err
	}

	// Get most recent model and skill from match_player_stats
	var model sql.NullString
	var skill sql.NullFloat64
	_ = s.db.QueryRowContext(ctx, `
		SELECT mps.model, mps.skill
		FROM match_player_stats mps
		JOIN player_guids pg ON mps.player_guid_id = pg.id
		JOIN matches m ON mps.match_id = m.id
		WHERE pg.player_id = ? AND mps.model IS NOT NULL AND mps.model != ''
		ORDER BY m.ended_at DESC
		LIMIT 1
	`, id).Scan(&model, &skill)
	if model.Valid {
		p.Model = model.String
	}
	if skill.Valid {
		p.Skill = skill.Float64
	}

	// Get all GUIDs for this player
	guids, err := s.GetPlayerGUIDs(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.GUIDs = guids

	return &p, nil
}

// SearchPlayers searches for players by name (and optionally by GUID for admins)
func (s *Store) SearchPlayers(ctx context.Context, query string, limit int, includeGUID bool) ([]domain.Player, error) {
	if limit <= 0 {
		limit = 20
	}
	searchPattern := "%" + query + "%"

	var rows *sql.Rows
	var err error

	if includeGUID {
		// Search by name OR by GUID (admin feature)
		rows, err = s.db.QueryContext(ctx, `
			SELECT DISTINCT p.id, p.name, p.clean_name, p.first_seen, p.last_seen,
				COALESCE((
					SELECT SUM(s.duration_seconds)
					FROM sessions s
					JOIN player_guids pg2 ON s.player_guid_id = pg2.id
					WHERE pg2.player_id = p.id AND s.left_at IS NOT NULL
				), 0) as total_playtime_seconds,
				p.is_bot, p.is_vr
			FROM players p
			LEFT JOIN player_guids pg ON pg.player_id = p.id
			WHERE p.clean_name LIKE ? OR p.name LIKE ? OR pg.guid LIKE ?
			ORDER BY p.last_seen DESC
			LIMIT ?
		`, searchPattern, searchPattern, searchPattern, limit)
	} else {
		// Search by name only
		rows, err = s.db.QueryContext(ctx, `
			SELECT p.id, p.name, p.clean_name, p.first_seen, p.last_seen,
				COALESCE((
					SELECT SUM(s.duration_seconds)
					FROM sessions s
					JOIN player_guids pg ON s.player_guid_id = pg.id
					WHERE pg.player_id = p.id AND s.left_at IS NOT NULL
				), 0) as total_playtime_seconds,
				p.is_bot, p.is_vr
			FROM players p
			WHERE p.clean_name LIKE ? OR p.name LIKE ?
			ORDER BY p.last_seen DESC
			LIMIT ?
		`, searchPattern, searchPattern, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []domain.Player
	for rows.Next() {
		var p domain.Player
		if err := rows.Scan(&p.ID, &p.Name, &p.CleanName, &p.FirstSeen, &p.LastSeen, &p.TotalPlaytimeSeconds, &p.IsBot, &p.IsVR); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

// GetPlayers returns players with pagination support
func (s *Store) GetPlayers(ctx context.Context, limit, offset int) ([]domain.Player, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM players`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.name, p.clean_name, p.first_seen, p.last_seen,
			COALESCE((
				SELECT SUM(s.duration_seconds)
				FROM sessions s
				JOIN player_guids pg ON s.player_guid_id = pg.id
				WHERE pg.player_id = p.id AND s.left_at IS NOT NULL
			), 0) as total_playtime_seconds,
			p.is_bot, p.is_vr
		FROM players p ORDER BY p.last_seen DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var players []domain.Player
	for rows.Next() {
		var p domain.Player
		if err := rows.Scan(&p.ID, &p.Name, &p.CleanName, &p.FirstSeen, &p.LastSeen, &p.TotalPlaytimeSeconds, &p.IsBot, &p.IsVR); err != nil {
			return nil, 0, err
		}
		players = append(players, p)
	}
	return players, total, rows.Err()
}

// RecordPlayerName records a name in the history for a player_guid
func (s *Store) RecordPlayerName(ctx context.Context, playerGUIDID int64, name, cleanName string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO player_names (player_guid_id, name, clean_name, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(player_guid_id, clean_name) DO UPDATE SET
			name = excluded.name,
			last_seen = excluded.last_seen
	`, playerGUIDID, name, cleanName, formatTimestamp(now), formatTimestamp(now))
	return err
}

// GetPlayerNames returns all known names for a player (across all GUIDs)
func (s *Store) GetPlayerNames(ctx context.Context, playerID int64) ([]domain.PlayerName, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pn.name, pn.clean_name, pn.first_seen, pn.last_seen
		FROM player_names pn
		JOIN player_guids pg ON pn.player_guid_id = pg.id
		WHERE pg.player_id = ?
		ORDER BY pn.last_seen DESC
	`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []domain.PlayerName
	for rows.Next() {
		var pn domain.PlayerName
		if err := rows.Scan(&pn.Name, &pn.CleanName, &pn.FirstSeen, &pn.LastSeen); err != nil {
			return nil, err
		}
		names = append(names, pn)
	}
	return names, rows.Err()
}

// --- Player Merge/Link methods ---

// MergePlayers moves all GUIDs from sourcePlayerID to targetPlayerID, then deletes source
func (s *Store) MergePlayers(ctx context.Context, targetPlayerID, sourcePlayerID int64) error {
	// Move all GUIDs to target player
	_, err := s.db.ExecContext(ctx, `
		UPDATE player_guids SET player_id = ? WHERE player_id = ?
	`, targetPlayerID, sourcePlayerID)
	if err != nil {
		return err
	}

	// Update target player's first_seen, last_seen, and recompute is_vr
	// Note: name/clean_name are NOT updated here - we preserve the target player's name
	// The name will update naturally when any of the merged GUIDs become active again
	_, err = s.db.ExecContext(ctx, `
		UPDATE players SET
			first_seen = (SELECT MIN(first_seen) FROM player_guids WHERE player_id = ?),
			last_seen = (SELECT MAX(last_seen) FROM player_guids WHERE player_id = ?),
			is_vr = EXISTS(SELECT 1 FROM player_guids WHERE player_id = ? AND is_vr = TRUE)
		WHERE id = ?
	`, targetPlayerID, targetPlayerID, targetPlayerID, targetPlayerID)
	if err != nil {
		return err
	}

	// Delete the source player (CASCADE will handle if any orphaned refs)
	_, err = s.db.ExecContext(ctx, `DELETE FROM players WHERE id = ?`, sourcePlayerID)
	return err
}

// SplitGUID creates a new player from a GUID (for unlinking)
func (s *Store) SplitGUID(ctx context.Context, playerGUIDID int64) (*domain.Player, error) {
	// Get the GUID info
	var pg domain.PlayerGUID
	err := s.db.QueryRowContext(ctx, `
		SELECT id, player_id, guid, name, clean_name, first_seen, last_seen, is_vr
		FROM player_guids WHERE id = ?
	`, playerGUIDID).Scan(&pg.ID, &pg.PlayerID, &pg.GUID, &pg.Name, &pg.CleanName, &pg.FirstSeen, &pg.LastSeen, &pg.IsVR)
	if err != nil {
		return nil, err
	}

	// Check if this is the only GUID for the player
	var guidCount int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM player_guids WHERE player_id = ?
	`, pg.PlayerID).Scan(&guidCount)
	if err != nil {
		return nil, err
	}
	if guidCount <= 1 {
		return nil, fmt.Errorf("cannot split: player only has one GUID")
	}

	// Create new player (inherit is_vr from the GUID being split)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO players (name, clean_name, first_seen, last_seen, is_vr)
		VALUES (?, ?, ?, ?, ?)
	`, pg.Name, pg.CleanName, formatTimestamp(pg.FirstSeen), formatTimestamp(pg.LastSeen), pg.IsVR)
	if err != nil {
		return nil, err
	}
	newPlayerID, _ := result.LastInsertId()

	// Move the GUID to new player
	_, err = s.db.ExecContext(ctx, `
		UPDATE player_guids SET player_id = ? WHERE id = ?
	`, newPlayerID, playerGUIDID)
	if err != nil {
		return nil, err
	}

	// Recompute source player's is_vr from remaining GUIDs
	_, err = s.db.ExecContext(ctx, `
		UPDATE players SET is_vr = EXISTS(
			SELECT 1 FROM player_guids WHERE player_id = ? AND is_vr = TRUE
		) WHERE id = ?
	`, pg.PlayerID, pg.PlayerID)
	if err != nil {
		return nil, err
	}

	// Return the new player
	return s.GetPlayerByID(ctx, newPlayerID)
}

// --- Session methods ---

// CreateSession starts a new player session
func (s *Store) CreateSession(ctx context.Context, sess *domain.Session) error {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (player_guid_id, server_id, joined_at, ip_address)
		VALUES (?, ?, ?, ?)
	`, sess.PlayerGUIDID, sess.ServerID, formatTimestamp(sess.JoinedAt), sess.IPAddress)
	if err != nil {
		return err
	}
	sess.ID, _ = result.LastInsertId()
	return nil
}

// EndSession closes a session with the leave time (idempotent - no-op if already closed)
func (s *Store) EndSession(ctx context.Context, sessionID int64, leftAt time.Time) error {
	formattedLeftAt := formatTimestamp(leftAt)
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET
			left_at = ?,
			duration_seconds = CAST((julianday(?) - julianday(joined_at)) * 86400 AS INTEGER)
		WHERE id = ? AND left_at IS NULL
	`, formattedLeftAt, formattedLeftAt, sessionID)
	return err
}

// GetSessionByPlayerAndJoinTime finds a session by exact start time (for replay idempotency)
func (s *Store) GetSessionByPlayerAndJoinTime(ctx context.Context, playerGUIDID, serverID int64, joinedAt time.Time) (*domain.Session, error) {
	var sess domain.Session
	var leftAt sql.NullTime
	var durationSeconds sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, player_guid_id, server_id, joined_at, left_at, duration_seconds
		FROM sessions
		WHERE player_guid_id = ? AND server_id = ? AND joined_at = ?
	`, playerGUIDID, serverID, formatTimestamp(joinedAt)).Scan(&sess.ID, &sess.PlayerGUIDID, &sess.ServerID, &sess.JoinedAt, &leftAt, &durationSeconds)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if leftAt.Valid {
		sess.LeftAt = &leftAt.Time
	}
	if durationSeconds.Valid {
		sess.DurationSeconds = durationSeconds.Int64
	}
	return &sess, nil
}

// GetSessionActiveAt finds a closed session that was active at the given timestamp (for replay idempotency)
// This handles the case where a ClientBegin occurs mid-session (e.g., map change) during replay
func (s *Store) GetSessionActiveAt(ctx context.Context, playerGUIDID, serverID int64, timestamp time.Time) (*domain.Session, error) {
	var sess domain.Session
	var leftAt sql.NullTime
	var durationSeconds sql.NullInt64
	ts := formatTimestamp(timestamp)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, player_guid_id, server_id, joined_at, left_at, duration_seconds
		FROM sessions
		WHERE player_guid_id = ? AND server_id = ? AND joined_at <= ? AND left_at >= ?
		ORDER BY joined_at DESC LIMIT 1
	`, playerGUIDID, serverID, ts, ts).Scan(&sess.ID, &sess.PlayerGUIDID, &sess.ServerID, &sess.JoinedAt, &leftAt, &durationSeconds)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if leftAt.Valid {
		sess.LeftAt = &leftAt.Time
	}
	if durationSeconds.Valid {
		sess.DurationSeconds = durationSeconds.Int64
	}
	return &sess, nil
}

// GetOpenSessionForPlayer finds an open session (left_at IS NULL) for a player on a server
func (s *Store) GetOpenSessionForPlayer(ctx context.Context, playerGUIDID, serverID int64) (*domain.Session, error) {
	var sess domain.Session
	err := s.db.QueryRowContext(ctx, `
		SELECT id, player_guid_id, server_id, joined_at
		FROM sessions
		WHERE player_guid_id = ? AND server_id = ? AND left_at IS NULL
		ORDER BY joined_at DESC LIMIT 1
	`, playerGUIDID, serverID).Scan(&sess.ID, &sess.PlayerGUIDID, &sess.ServerID, &sess.JoinedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// EndOpenSessionsBefore closes all open sessions that started before a timestamp
// Used for both clean shutdown and crash recovery to avoid closing sessions from later events during replay
func (s *Store) EndOpenSessionsBefore(ctx context.Context, serverID int64, before, leftAt time.Time) error {
	formattedLeftAt := formatTimestamp(leftAt)
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET
			left_at = ?,
			duration_seconds = CAST((julianday(?) - julianday(joined_at)) * 86400 AS INTEGER)
		WHERE server_id = ? AND joined_at < ? AND left_at IS NULL
	`, formattedLeftAt, formattedLeftAt, serverID, formatTimestamp(before))
	return err
}

// GetActiveSessions returns sessions without a leave time for a server
func (s *Store) GetActiveSessions(ctx context.Context, serverID int64) ([]domain.Session, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, player_guid_id, server_id, joined_at
		FROM sessions WHERE server_id = ? AND left_at IS NULL
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []domain.Session
	for rows.Next() {
		var sess domain.Session
		if err := rows.Scan(&sess.ID, &sess.PlayerGUIDID, &sess.ServerID, &sess.JoinedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

// --- Match methods ---

// CreateMatch starts a new match
func (s *Store) CreateMatch(ctx context.Context, m *domain.Match) error {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO matches (uuid, server_id, map_name, game_type, started_at)
		VALUES (?, ?, ?, ?, ?)
	`, m.UUID, m.ServerID, m.MapName, m.GameType, formatTimestamp(m.StartedAt))
	if err != nil {
		return err
	}
	m.ID, _ = result.LastInsertId()
	return nil
}

// GetMatchByUUID retrieves a match by its UUID
func (s *Store) GetMatchByUUID(ctx context.Context, uuid string) (*domain.Match, error) {
	if uuid == "" {
		return nil, nil
	}
	var m domain.Match
	var endedAt sql.NullTime
	var exitReason sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, uuid, server_id, map_name, game_type, started_at, ended_at, exit_reason
		FROM matches WHERE uuid = ?
	`, uuid).Scan(&m.ID, &m.UUID, &m.ServerID, &m.MapName, &m.GameType, &m.StartedAt, &endedAt, &exitReason)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if endedAt.Valid {
		m.EndedAt = &endedAt.Time
	}
	if exitReason.Valid {
		m.ExitReason = exitReason.String
	}
	return &m, nil
}

// EndMatch closes a match and updates the server's last match tracking for log replay
func (s *Store) EndMatch(ctx context.Context, matchID int64, endedAt time.Time, exitReason string, redScore, blueScore *int) error {
	formattedEndedAt := formatTimestamp(endedAt)
	_, err := s.db.ExecContext(ctx, `
		UPDATE matches SET ended_at = ?, exit_reason = ?, red_score = ?, blue_score = ?
		WHERE id = ? AND ended_at IS NULL
	`, formattedEndedAt, exitReason, redScore, blueScore, matchID)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE servers
		SET last_match_uuid = (SELECT uuid FROM matches WHERE id = ?),
		    last_match_ended_at = ?
		WHERE id = (SELECT server_id FROM matches WHERE id = ?)
	`, matchID, formattedEndedAt, matchID)
	return err
}

// UpdateMatchStartTime updates the match start time (used when warmup ends, only if not already ended)
func (s *Store) UpdateMatchStartTime(ctx context.Context, matchID int64, startedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE matches SET started_at = ?
		WHERE id = ? AND ended_at IS NULL
	`, formatTimestamp(startedAt), matchID)
	return err
}

// EndAllOpenMatches closes all open matches for a server, optionally excluding a match in progress
func (s *Store) EndAllOpenMatches(ctx context.Context, serverID int64, endedAt time.Time, exitReason string, matchInProgressID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE matches SET ended_at = ?, exit_reason = ?
		WHERE server_id = ? AND ended_at IS NULL AND id != ?
	`, formatTimestamp(endedAt), exitReason, serverID, matchInProgressID)
	return err
}

// GetCurrentMatch returns the most recent open (unended) match for a server
func (s *Store) GetCurrentMatch(ctx context.Context, serverID int64) (*domain.Match, error) {
	var m domain.Match
	var gameType sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, server_id, map_name, game_type, started_at
		FROM matches
		WHERE server_id = ? AND ended_at IS NULL
		ORDER BY started_at DESC
		LIMIT 1
	`, serverID).Scan(&m.ID, &m.ServerID, &m.MapName, &gameType, &m.StartedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.GameType = gameType.String
	return &m, nil
}

// GetRecentMatches returns recent completed matches
func (s *Store) GetRecentMatches(ctx context.Context, limit int) ([]domain.Match, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, server_id, map_name, game_type, started_at, ended_at, exit_reason
		FROM matches WHERE ended_at IS NOT NULL ORDER BY ended_at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []domain.Match
	for rows.Next() {
		var m domain.Match
		var endedAt sql.NullTime
		var exitReason sql.NullString
		if err := rows.Scan(&m.ID, &m.ServerID, &m.MapName, &m.GameType, &m.StartedAt, &endedAt, &exitReason); err != nil {
			return nil, err
		}
		if endedAt.Valid {
			m.EndedAt = &endedAt.Time
		}
		m.ExitReason = exitReason.String
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

// GetActiveMatch returns the current match for a server (no end time)
func (s *Store) GetActiveMatch(ctx context.Context, serverID int64) (*domain.Match, error) {
	var m domain.Match
	err := s.db.QueryRowContext(ctx, `
		SELECT id, server_id, map_name, game_type, started_at
		FROM matches WHERE server_id = ? AND ended_at IS NULL
		ORDER BY started_at DESC LIMIT 1
	`, serverID).Scan(&m.ID, &m.ServerID, &m.MapName, &m.GameType, &m.StartedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// --- Match Player Stats methods ---

// FlushMatchPlayerStats writes all accumulated stats for a player to the database.
// This is called at disconnect or match end, creating the row if it doesn't exist.
// For bots: uses full primary key (match_id, player_guid_id, client_id) allowing multiple bot instances
// For humans: one row per player per match, updates client_id on reconnect
func (s *Store) FlushMatchPlayerStats(ctx context.Context, matchID, playerGUIDID int64, clientID int,
	frags, deaths int, completed bool, score *int, team *int, model string, skill float64, victory bool,
	captures, flagReturns, assists, impressives, excellents, humiliations, defends int,
	isBot bool, joinedLate bool, joinedAt time.Time, isVR bool) error {

	if isBot {
		// Bots: upsert by full primary key (allows multiple same-GUID bots)
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO match_player_stats (
				match_id, player_guid_id, client_id, frags, deaths, completed, score, team,
				model, skill, victories, captures, flag_returns, assists, impressives,
				excellents, humiliations, defends, joined_late, joined_at, is_vr
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(match_id, player_guid_id, client_id) DO UPDATE SET
				frags = frags + excluded.frags,
				deaths = deaths + excluded.deaths,
				completed = completed OR excluded.completed,
				score = COALESCE(excluded.score, score),
				team = COALESCE(excluded.team, team),
				model = COALESCE(excluded.model, model),
				skill = COALESCE(excluded.skill, skill),
				victories = MAX(victories, excluded.victories),
				captures = captures + excluded.captures,
				flag_returns = flag_returns + excluded.flag_returns,
				assists = assists + excluded.assists,
				impressives = impressives + excluded.impressives,
				excellents = excellents + excluded.excellents,
				humiliations = humiliations + excluded.humiliations,
				defends = defends + excluded.defends,
				is_vr = is_vr OR excluded.is_vr
		`, matchID, playerGUIDID, clientID, frags, deaths, completed, score, team,
			model, skill, boolToInt(victory), captures, flagReturns, assists, impressives,
			excellents, humiliations, defends, joinedLate, formatTimestamp(joinedAt), isVR)
		return err
	}

	// Humans: one row per player, update client_id on reconnect
	// First try to update existing row (handles reconnects with different client_id)
	result, err := s.db.ExecContext(ctx, `
		UPDATE match_player_stats SET
			client_id = ?,
			frags = frags + ?,
			deaths = deaths + ?,
			completed = completed OR ?,
			score = COALESCE(?, score),
			team = COALESCE(?, team),
			model = COALESCE(?, model),
			skill = COALESCE(?, skill),
			victories = MAX(victories, ?),
			captures = captures + ?,
			flag_returns = flag_returns + ?,
			assists = assists + ?,
			impressives = impressives + ?,
			excellents = excellents + ?,
			humiliations = humiliations + ?,
			defends = defends + ?,
			is_vr = is_vr OR ?
		WHERE match_id = ? AND player_guid_id = ?
	`, clientID, frags, deaths, completed, score, team, model, skill, boolToInt(victory),
		captures, flagReturns, assists, impressives, excellents, humiliations, defends,
		isVR, matchID, playerGUIDID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		return nil // Updated existing row
	}

	// No existing row - insert new
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO match_player_stats (
			match_id, player_guid_id, client_id, frags, deaths, completed, score, team,
			model, skill, victories, captures, flag_returns, assists, impressives,
			excellents, humiliations, defends, joined_late, joined_at, is_vr
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, matchID, playerGUIDID, clientID, frags, deaths, completed, score, team,
		model, skill, boolToInt(victory), captures, flagReturns, assists, impressives,
		excellents, humiliations, defends, joinedLate, formatTimestamp(joinedAt), isVR)
	if err != nil {
		return err
	}

	// Mark match as having a human player
	_, err = s.db.ExecContext(ctx, `UPDATE matches SET has_human_player = TRUE WHERE id = ? AND has_human_player = FALSE`, matchID)
	if err != nil {
		return err
	}

	// Propagate VR status to player_guids and players (sticky: never reset to false)
	if isVR {
		_, err = s.db.ExecContext(ctx, `UPDATE player_guids SET is_vr = TRUE WHERE id = ? AND is_vr = FALSE`, playerGUIDID)
		if err != nil {
			return err
		}
		_, err = s.db.ExecContext(ctx, `
			UPDATE players SET is_vr = TRUE
			WHERE id = (SELECT player_id FROM player_guids WHERE id = ?) AND is_vr = FALSE
		`, playerGUIDID)
		if err != nil {
			return err
		}
	}

	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// --- Stats methods ---

// GetLeaderboard returns top players ranked by the specified category and time period
func (s *Store) GetLeaderboard(ctx context.Context, category, period string, limit int, gameType string) (*domain.LeaderboardResponse, error) {
	start, end := getTimePeriodBounds(period)

	// Determine ORDER BY clause based on category
	var orderBy string
	switch category {
	case "kd_ratio":
		orderBy = "kd_ratio DESC"
	case "deaths":
		orderBy = "total_deaths DESC"
	case "captures":
		orderBy = "total_captures DESC"
	case "matches":
		orderBy = "completed_matches DESC"
	case "assists":
		orderBy = "total_assists DESC"
	case "impressives":
		orderBy = "total_impressives DESC"
	case "excellents":
		orderBy = "total_excellents DESC"
	case "humiliations":
		orderBy = "total_humiliations DESC"
	case "defends":
		orderBy = "total_defends DESC"
	case "flag_returns":
		orderBy = "total_flag_returns DESC"
	case "victories":
		orderBy = "total_victories DESC"
	default: // "frags"
		orderBy = "total_frags DESC"
	}

	havingClause := "HAVING completed_matches >= 5"

	var query string
	var args []interface{}

	// Always join matches if filtering by game type or period
	needsMatchJoin := period != "all" || gameType != ""

	if !needsMatchJoin {
		query = `
			SELECT
				p.id, p.name, p.clean_name, p.first_seen, p.last_seen,
				COALESCE((
					SELECT SUM(s.duration_seconds)
					FROM sessions s
					JOIN player_guids pg3 ON s.player_guid_id = pg3.id
					WHERE pg3.player_id = p.id AND s.left_at IS NOT NULL
				), 0) as total_playtime_seconds,
				p.is_bot, p.is_vr,
				COALESCE(SUM(mps.frags), 0) as total_frags,
				COALESCE(SUM(mps.deaths), 0) as total_deaths,
				COUNT(DISTINCT mps.match_id) as total_matches,
				COUNT(DISTINCT CASE WHEN mps.completed = 1 THEN mps.match_id END) as completed_matches,
				COUNT(DISTINCT CASE WHEN mps.completed = 0 THEN mps.match_id END) as uncompleted_matches,
				COALESCE(SUM(mps.captures), 0) as total_captures,
				COALESCE(SUM(mps.flag_returns), 0) as total_flag_returns,
				COALESCE(SUM(mps.assists), 0) as total_assists,
				COALESCE(SUM(mps.impressives), 0) as total_impressives,
				COALESCE(SUM(mps.excellents), 0) as total_excellents,
				COALESCE(SUM(mps.humiliations), 0) as total_humiliations,
				COALESCE(SUM(mps.defends), 0) as total_defends,
				COALESCE(SUM(mps.victories), 0) as total_victories,
				CASE WHEN SUM(mps.deaths) > 0
					THEN CAST(SUM(mps.frags) AS REAL) / SUM(mps.deaths)
					ELSE COALESCE(SUM(mps.frags), 0) END as kd_ratio,
				(SELECT mps2.model FROM match_player_stats mps2
					JOIN player_guids pg2 ON mps2.player_guid_id = pg2.id
					JOIN matches m2 ON mps2.match_id = m2.id
					WHERE pg2.player_id = p.id AND mps2.model IS NOT NULL AND mps2.model != ''
					ORDER BY m2.ended_at DESC LIMIT 1) as model,
				(SELECT mps2.skill FROM match_player_stats mps2
					JOIN player_guids pg2 ON mps2.player_guid_id = pg2.id
					JOIN matches m2 ON mps2.match_id = m2.id
					WHERE pg2.player_id = p.id AND mps2.skill IS NOT NULL
					ORDER BY m2.ended_at DESC LIMIT 1) as skill
			FROM players p
			JOIN player_guids pg ON p.id = pg.player_id
			LEFT JOIN match_player_stats mps ON pg.id = mps.player_guid_id
			WHERE p.is_bot = FALSE AND p.clean_name NOT LIKE '[VR] Player#%'
			GROUP BY p.id
			` + havingClause + `
			ORDER BY ` + orderBy + `
			LIMIT ?`
		args = []interface{}{limit}
	} else {
		// Build WHERE conditions
		whereConditions := "p.is_bot = FALSE AND p.clean_name NOT LIKE '[VR] Player#%'"

		if period != "all" {
			whereConditions += " AND m.started_at >= ? AND m.started_at < ?"
			args = append(args, formatTimestamp(start), formatTimestamp(end))
		}

		if gameType != "" {
			whereConditions += " AND m.game_type = ?"
			args = append(args, gameType)
		}

		args = append(args, limit)

		query = `
			SELECT
				p.id, p.name, p.clean_name, p.first_seen, p.last_seen,
				COALESCE((
					SELECT SUM(s.duration_seconds)
					FROM sessions s
					JOIN player_guids pg3 ON s.player_guid_id = pg3.id
					WHERE pg3.player_id = p.id AND s.left_at IS NOT NULL
				), 0) as total_playtime_seconds,
				p.is_bot, p.is_vr,
				COALESCE(SUM(mps.frags), 0) as total_frags,
				COALESCE(SUM(mps.deaths), 0) as total_deaths,
				COUNT(DISTINCT mps.match_id) as total_matches,
				COUNT(DISTINCT CASE WHEN mps.completed = 1 THEN mps.match_id END) as completed_matches,
				COUNT(DISTINCT CASE WHEN mps.completed = 0 THEN mps.match_id END) as uncompleted_matches,
				COALESCE(SUM(mps.captures), 0) as total_captures,
				COALESCE(SUM(mps.flag_returns), 0) as total_flag_returns,
				COALESCE(SUM(mps.assists), 0) as total_assists,
				COALESCE(SUM(mps.impressives), 0) as total_impressives,
				COALESCE(SUM(mps.excellents), 0) as total_excellents,
				COALESCE(SUM(mps.humiliations), 0) as total_humiliations,
				COALESCE(SUM(mps.defends), 0) as total_defends,
				COALESCE(SUM(mps.victories), 0) as total_victories,
				CASE WHEN SUM(mps.deaths) > 0
					THEN CAST(SUM(mps.frags) AS REAL) / SUM(mps.deaths)
					ELSE COALESCE(SUM(mps.frags), 0) END as kd_ratio,
				(SELECT mps2.model FROM match_player_stats mps2
					JOIN player_guids pg2 ON mps2.player_guid_id = pg2.id
					JOIN matches m2 ON mps2.match_id = m2.id
					WHERE pg2.player_id = p.id AND mps2.model IS NOT NULL AND mps2.model != ''
					ORDER BY m2.ended_at DESC LIMIT 1) as model,
				(SELECT mps2.skill FROM match_player_stats mps2
					JOIN player_guids pg2 ON mps2.player_guid_id = pg2.id
					JOIN matches m2 ON mps2.match_id = m2.id
					WHERE pg2.player_id = p.id AND mps2.skill IS NOT NULL
					ORDER BY m2.ended_at DESC LIMIT 1) as skill
			FROM players p
			JOIN player_guids pg ON p.id = pg.player_id
			LEFT JOIN match_player_stats mps ON pg.id = mps.player_guid_id
			LEFT JOIN matches m ON mps.match_id = m.id
			WHERE ` + whereConditions + `
			GROUP BY p.id
			` + havingClause + `
			ORDER BY ` + orderBy + `
			LIMIT ?`
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]domain.LeaderboardEntry, 0)
	rank := 0
	for rows.Next() {
		rank++
		var e domain.LeaderboardEntry
		var model sql.NullString
		var skill sql.NullFloat64
		if err := rows.Scan(
			&e.Player.ID, &e.Player.Name, &e.Player.CleanName,
			&e.Player.FirstSeen, &e.Player.LastSeen, &e.Player.TotalPlaytimeSeconds, &e.Player.IsBot, &e.Player.IsVR,
			&e.TotalFrags, &e.TotalDeaths, &e.TotalMatches, &e.CompletedMatches, &e.UncompletedMatches,
			&e.Captures, &e.FlagReturns, &e.Assists, &e.Impressives, &e.Excellents,
			&e.Humiliations, &e.Defends, &e.Victories,
			&e.KDRatio, &model, &skill,
		); err != nil {
			return nil, err
		}
		if model.Valid {
			e.Player.Model = model.String
		}
		if skill.Valid {
			e.Player.Skill = skill.Float64
		}
		e.Rank = rank
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	response := &domain.LeaderboardResponse{
		Category: category,
		Period:   period,
		Entries:  entries,
	}
	if period != "all" {
		response.PeriodStart = &start
		response.PeriodEnd = &end
	}
	return response, nil
}

// getTimePeriodBounds returns start and end times for a given period (rolling windows)
func getTimePeriodBounds(period string) (start, end time.Time) {
	now := time.Now()
	end = now
	switch period {
	case "day":
		start = now.Add(-24 * time.Hour)
	case "week":
		start = now.Add(-7 * 24 * time.Hour)
	case "month":
		start = now.Add(-30 * 24 * time.Hour)
	case "year":
		start = now.Add(-365 * 24 * time.Hour)
	default: // "all"
		start = time.Time{}
		end = now.Add(100 * 365 * 24 * time.Hour)
	}
	return
}

// GetPlayerStatsByID returns aggregated stats for a player by ID with time filtering
func (s *Store) GetPlayerStatsByID(ctx context.Context, playerID int64, period string) (*domain.PlayerStatsResponse, error) {
	return s.getPlayerStats(ctx, playerID, period)
}

// getPlayerStats is the shared implementation for getting player stats
func (s *Store) getPlayerStats(ctx context.Context, playerID int64, period string) (*domain.PlayerStatsResponse, error) {
	// Get player with GUIDs
	player, err := s.GetPlayerByID(ctx, playerID)
	if err != nil {
		return nil, err
	}

	start, end := getTimePeriodBounds(period)

	// Query aggregated stats across all GUIDs
	var stats domain.AggregatedStats
	var query string
	var args []interface{}

	if period == "all" {
		query = `
			SELECT
				COUNT(DISTINCT mps.match_id) as matches,
				COUNT(DISTINCT CASE WHEN mps.completed = 1 THEN mps.match_id END) as completed_matches,
				COUNT(DISTINCT CASE WHEN mps.completed = 0 THEN mps.match_id END) as uncompleted_matches,
				COALESCE(SUM(mps.frags), 0) as frags,
				COALESCE(SUM(mps.deaths), 0) as deaths,
				COALESCE(SUM(mps.captures), 0) as captures,
				COALESCE(SUM(mps.flag_returns), 0) as flag_returns,
				COALESCE(SUM(mps.assists), 0) as assists,
				COALESCE(SUM(mps.impressives), 0) as impressives,
				COALESCE(SUM(mps.excellents), 0) as excellents,
				COALESCE(SUM(mps.humiliations), 0) as humiliations,
				COALESCE(SUM(mps.defends), 0) as defends,
				COALESCE(SUM(mps.victories), 0) as victories
			FROM match_player_stats mps
			JOIN player_guids pg ON mps.player_guid_id = pg.id
			WHERE pg.player_id = ?
		`
		args = []interface{}{playerID}
	} else {
		query = `
			SELECT
				COUNT(DISTINCT mps.match_id) as matches,
				COUNT(DISTINCT CASE WHEN mps.completed = 1 THEN mps.match_id END) as completed_matches,
				COUNT(DISTINCT CASE WHEN mps.completed = 0 THEN mps.match_id END) as uncompleted_matches,
				COALESCE(SUM(mps.frags), 0) as frags,
				COALESCE(SUM(mps.deaths), 0) as deaths,
				COALESCE(SUM(mps.captures), 0) as captures,
				COALESCE(SUM(mps.flag_returns), 0) as flag_returns,
				COALESCE(SUM(mps.assists), 0) as assists,
				COALESCE(SUM(mps.impressives), 0) as impressives,
				COALESCE(SUM(mps.excellents), 0) as excellents,
				COALESCE(SUM(mps.humiliations), 0) as humiliations,
				COALESCE(SUM(mps.defends), 0) as defends,
				COALESCE(SUM(mps.victories), 0) as victories
			FROM match_player_stats mps
			JOIN player_guids pg ON mps.player_guid_id = pg.id
			JOIN matches m ON mps.match_id = m.id
			WHERE pg.player_id = ?
			  AND m.started_at >= ?
			  AND m.started_at < ?
		`
		args = []interface{}{playerID, formatTimestamp(start), formatTimestamp(end)}
	}

	err = s.db.QueryRowContext(ctx, query, args...).Scan(
		&stats.Matches, &stats.CompletedMatches, &stats.UncompletedMatches,
		&stats.Frags, &stats.Deaths,
		&stats.Captures, &stats.FlagReturns, &stats.Assists,
		&stats.Impressives, &stats.Excellents,
		&stats.Humiliations, &stats.Defends, &stats.Victories,
	)
	if err != nil {
		return nil, err
	}

	// Calculate K/D ratio
	if stats.Deaths > 0 {
		stats.KDRatio = float64(stats.Frags) / float64(stats.Deaths)
	} else if stats.Frags > 0 {
		stats.KDRatio = float64(stats.Frags)
	}

	// Get name history
	names, err := s.GetPlayerNames(ctx, playerID)
	if err != nil {
		return nil, err
	}

	response := &domain.PlayerStatsResponse{
		Player: *player,
		Period: period,
		Stats:  stats,
		Names:  names,
	}

	// Include period bounds for non-"all" periods
	if period != "all" {
		response.PeriodStart = &start
		response.PeriodEnd = &end
	}

	return response, nil
}

// --- User methods ---

// User represents an authenticated user
type User struct {
	ID                     int64
	Username               string
	PasswordHash           string
	IsAdmin                bool
	PlayerID               *int64
	PasswordChangeRequired bool
	CreatedAt              time.Time
	LastLogin              *time.Time
}

// CreateUser creates a new user account
func (s *Store) CreateUser(ctx context.Context, username, passwordHash string, isAdmin bool, playerID *int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, is_admin, player_id, password_change_required)
		VALUES (?, ?, ?, ?, TRUE)
	`, username, passwordHash, isAdmin, playerID)
	return err
}

// GetUserByUsername retrieves a user by username
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, is_admin, player_id, password_change_required, created_at, last_login
		FROM users WHERE username = ?
	`, username)
	return scanUser(row)
}

// GetUserByID retrieves a user by ID
func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, is_admin, player_id, password_change_required, created_at, last_login
		FROM users WHERE id = ?
	`, id)
	return scanUser(row)
}

// DeleteUser removes a user by username
func (s *Store) DeleteUser(ctx context.Context, username string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE username = ?`, username)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("user not found: %s", username)
	}
	return nil
}

// ListUsers returns all users with details
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, password_hash, is_admin, player_id, password_change_required, created_at, last_login
		FROM users ORDER BY username
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	return users, rows.Err()
}

// UpdateUserLastLogin updates the last login timestamp
func (s *Store) UpdateUserLastLogin(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET last_login = CURRENT_TIMESTAMP WHERE id = ?
	`, userID)
	return err
}

// UpdateUserPassword updates a user's password and clears the password_change_required flag
func (s *Store) UpdateUserPassword(ctx context.Context, userID int64, newPasswordHash string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET password_hash = ?, password_change_required = FALSE WHERE id = ?
	`, newPasswordHash, userID)
	return err
}

// ResetUserPassword sets a new temporary password (admin action)
func (s *Store) ResetUserPassword(ctx context.Context, userID int64, newPasswordHash string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET password_hash = ?, password_change_required = TRUE WHERE id = ?
	`, newPasswordHash, userID)
	return err
}

// IsPlayerClaimed checks if a player is already linked to a user
func (s *Store) IsPlayerClaimed(ctx context.Context, playerID int64) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM users WHERE player_id = ?
	`, playerID).Scan(&count)
	return count > 0, err
}

// UpdateUserPlayerLink links or unlinks a player to a user
func (s *Store) UpdateUserPlayerLink(ctx context.Context, userID int64, playerID *int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET player_id = ? WHERE id = ?
	`, playerID, userID)
	return err
}

// UpdateUserAdmin updates the admin status of a user
func (s *Store) UpdateUserAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET is_admin = ? WHERE id = ?
	`, isAdmin, userID)
	return err
}

// VerifiedPlayer represents a player linked to a user account
type VerifiedPlayer struct {
	PlayerID  int64  `json:"player_id"`
	CleanName string `json:"clean_name"`
	IsAdmin   bool   `json:"is_admin"`
}

// GetVerifiedPlayers returns all players linked to user accounts
func (s *Store) GetVerifiedPlayers(ctx context.Context) ([]VerifiedPlayer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.player_id, p.clean_name, u.is_admin
		FROM users u
		JOIN players p ON p.id = u.player_id
		WHERE u.player_id IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []VerifiedPlayer
	for rows.Next() {
		var p VerifiedPlayer
		if err := rows.Scan(&p.PlayerID, &p.CleanName, &p.IsAdmin); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

// attachPlayersToMatches loads player stats for a list of matches and attaches them
func (s *Store) attachPlayersToMatches(ctx context.Context, matches []domain.MatchSummary, matchIDs []int64) ([]domain.MatchSummary, error) {
	if len(matchIDs) == 0 {
		return matches, nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(matchIDs))
	args := make([]interface{}, len(matchIDs))
	for i, id := range matchIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	// Get player stats for all matches
	playerRows, err := s.db.QueryContext(ctx, `
		SELECT mps.match_id, p.id, pg.name, pg.clean_name, mps.frags, mps.deaths, mps.completed, p.is_bot, mps.skill, mps.score, mps.team, mps.model, mps.impressives, mps.excellents, mps.humiliations, mps.defends, mps.victories, mps.captures, mps.assists, mps.is_vr
		FROM match_player_stats mps
		JOIN player_guids pg ON mps.player_guid_id = pg.id
		JOIN players p ON pg.player_id = p.id
		WHERE mps.match_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY mps.score DESC NULLS LAST, mps.frags DESC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer playerRows.Close()

	// Map players to matches
	playersByMatch := make(map[int64][]domain.MatchPlayerSummary)
	for playerRows.Next() {
		matchID, ps, err := scanMatchPlayerSummary(playerRows, true)
		if err != nil {
			return nil, err
		}
		playersByMatch[matchID] = append(playersByMatch[matchID], *ps)
	}
	if err := playerRows.Err(); err != nil {
		return nil, err
	}

	// Attach players to matches
	for i := range matches {
		matches[i].Players = playersByMatch[matches[i].ID]
	}

	return matches, nil
}

// GetRecentMatchSummaries returns recent finished matches with server and player info
func (s *Store) GetRecentMatchSummaries(ctx context.Context, limit int) ([]domain.MatchSummary, error) {
	// Get finished matches that have at least one player
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT
			m.id, m.server_id, s.name, m.map_name, m.game_type, m.started_at, m.ended_at, m.exit_reason,
			m.red_score, m.blue_score
		FROM matches m
		JOIN servers s ON m.server_id = s.id
		JOIN match_player_stats mps ON m.id = mps.match_id
		WHERE m.ended_at IS NOT NULL
		ORDER BY m.ended_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []domain.MatchSummary
	var matchIDs []int64
	for rows.Next() {
		m, err := scanMatchSummaryRow(rows)
		if err != nil {
			return nil, err
		}
		matches = append(matches, *m)
		matchIDs = append(matchIDs, m.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return s.attachPlayersToMatches(ctx, matches, matchIDs)
}

// GetPlayerRecentMatches returns recent finished matches that a specific player participated in
func (s *Store) GetPlayerRecentMatches(ctx context.Context, playerID int64, limit int, beforeID *int64) ([]domain.MatchSummary, error) {
	query := `
		SELECT DISTINCT
			m.id, m.server_id, s.name, m.map_name, m.game_type, m.started_at, m.ended_at, m.exit_reason,
			m.red_score, m.blue_score
		FROM matches m
		JOIN servers s ON m.server_id = s.id
		JOIN match_player_stats mps ON m.id = mps.match_id
		JOIN player_guids pg ON mps.player_guid_id = pg.id
		WHERE m.ended_at IS NOT NULL AND pg.player_id = ?`

	args := []interface{}{playerID}

	if beforeID != nil {
		query += ` AND m.id < ?`
		args = append(args, *beforeID)
	}

	query += ` ORDER BY m.ended_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []domain.MatchSummary
	var matchIDs []int64
	for rows.Next() {
		m, err := scanMatchSummaryRow(rows)
		if err != nil {
			return nil, err
		}
		matches = append(matches, *m)
		matchIDs = append(matchIDs, m.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return s.attachPlayersToMatches(ctx, matches, matchIDs)
}

// --- Link Code methods ---

// LinkCode represents a pending account link code
type LinkCode struct {
	ID         int64
	Code       string
	UserID     int64
	PlayerID   int64
	CreatedAt  time.Time
	ExpiresAt  time.Time
	UsedAt     *time.Time
	UsedByGUID *string
}

// generateLinkCode creates a secure 6-digit numeric code
func generateLinkCode() (string, error) {
	const digits = "0123456789"
	code := make([]byte, 6)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		code[i] = digits[n.Int64()]
	}
	return string(code), nil
}

// CreateLinkCode generates and stores a new link code
func (s *Store) CreateLinkCode(ctx context.Context, userID, playerID int64, expiresAt time.Time) (*LinkCode, error) {
	// Ensure uniqueness by retrying on conflict
	for attempts := 0; attempts < 5; attempts++ {
		code, err := generateLinkCode()
		if err != nil {
			return nil, fmt.Errorf("generating code: %w", err)
		}

		result, err := s.db.ExecContext(ctx, `
			INSERT INTO link_codes (code, user_id, player_id, expires_at)
			VALUES (?, ?, ?, ?)
		`, code, userID, playerID, expiresAt.UTC().Format("2006-01-02 15:04:05"))
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				continue
			}
			return nil, err
		}
		id, _ := result.LastInsertId()
		return &LinkCode{
			ID:        id,
			Code:      code,
			UserID:    userID,
			PlayerID:  playerID,
			ExpiresAt: expiresAt,
		}, nil
	}
	return nil, fmt.Errorf("failed to generate unique code after 5 attempts")
}

// GetValidLinkCode retrieves a valid (unexpired, unused) link code
func (s *Store) GetValidLinkCode(ctx context.Context, code string) (*LinkCode, error) {
	var lc LinkCode
	err := s.db.QueryRowContext(ctx, `
		SELECT id, code, user_id, player_id, created_at, expires_at
		FROM link_codes
		WHERE code = ? AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP
	`, code).Scan(&lc.ID, &lc.Code, &lc.UserID, &lc.PlayerID, &lc.CreatedAt, &lc.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &lc, nil
}

// MarkLinkCodeUsed marks a link code as used (atomically)
func (s *Store) MarkLinkCodeUsed(ctx context.Context, codeID int64, usedByGUID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE link_codes
		SET used_at = CURRENT_TIMESTAMP, used_by_guid = ?
		WHERE id = ? AND used_at IS NULL
	`, usedByGUID, codeID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("code already used or not found")
	}
	return nil
}

// CleanupExpiredLinkCodes removes expired codes
func (s *Store) CleanupExpiredLinkCodes(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM link_codes WHERE expires_at < CURRENT_TIMESTAMP
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// InvalidateUserLinkCodes invalidates all pending codes for a user
func (s *Store) InvalidateUserLinkCodes(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM link_codes WHERE user_id = ? AND used_at IS NULL
	`, userID)
	return err
}

// GetMatchSummaryByID returns a single match by ID with all player stats
func (s *Store) GetMatchSummaryByID(ctx context.Context, matchID int64) (*domain.MatchSummary, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT m.id, m.server_id, s.name, m.map_name, m.game_type, m.started_at, m.ended_at, m.exit_reason,
		       m.red_score, m.blue_score
		FROM matches m
		JOIN servers s ON m.server_id = s.id
		WHERE m.id = ?
	`, matchID)

	m, err := scanMatchSummaryRow(row)
	if err != nil {
		return nil, err
	}

	// Get player stats for this match
	playerRows, err := s.db.QueryContext(ctx, `
		SELECT p.id, pg.name, pg.clean_name, mps.frags, mps.deaths, mps.completed, p.is_bot, mps.skill, mps.score, mps.team, mps.model, mps.impressives, mps.excellents, mps.humiliations, mps.defends, mps.victories, mps.captures, mps.assists, mps.is_vr
		FROM match_player_stats mps
		JOIN player_guids pg ON mps.player_guid_id = pg.id
		JOIN players p ON pg.player_id = p.id
		WHERE mps.match_id = ?
		ORDER BY mps.score DESC NULLS LAST, mps.frags DESC
	`, matchID)
	if err != nil {
		return nil, err
	}
	defer playerRows.Close()

	for playerRows.Next() {
		_, ps, err := scanMatchPlayerSummary(playerRows, false)
		if err != nil {
			return nil, err
		}
		m.Players = append(m.Players, *ps)
	}
	if err := playerRows.Err(); err != nil {
		return nil, err
	}

	return m, nil
}

// MatchFilter defines filters for querying matches
type MatchFilter struct {
	GameType       string
	StartDate      *time.Time
	EndDate        *time.Time
	BeforeID       *int64
	Limit          int
	IncludeBotOnly bool // when false, filter to has_human_player = TRUE
}

// GetFilteredMatchSummaries returns matches filtered by the given criteria
func (s *Store) GetFilteredMatchSummaries(ctx context.Context, filter MatchFilter) ([]domain.MatchSummary, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}

	query := `
		SELECT DISTINCT
			m.id, m.server_id, s.name, m.map_name, m.game_type, m.started_at, m.ended_at, m.exit_reason,
			m.red_score, m.blue_score
		FROM matches m
		JOIN servers s ON m.server_id = s.id
		JOIN match_player_stats mps ON m.id = mps.match_id
		WHERE m.ended_at IS NOT NULL`

	var args []interface{}

	if filter.GameType != "" {
		query += ` AND m.game_type = ?`
		args = append(args, filter.GameType)
	}
	if filter.StartDate != nil {
		query += ` AND m.started_at >= ?`
		args = append(args, formatTimestamp(*filter.StartDate))
	}
	if filter.EndDate != nil {
		query += ` AND m.started_at <= ?`
		args = append(args, formatTimestamp(*filter.EndDate))
	}
	if filter.BeforeID != nil {
		query += ` AND m.id < ?`
		args = append(args, *filter.BeforeID)
	}
	if !filter.IncludeBotOnly {
		query += ` AND m.has_human_player = TRUE`
	}

	query += ` ORDER BY m.ended_at DESC LIMIT ?`
	args = append(args, filter.Limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []domain.MatchSummary
	var matchIDs []int64
	for rows.Next() {
		m, err := scanMatchSummaryRow(rows)
		if err != nil {
			return nil, err
		}
		matches = append(matches, *m)
		matchIDs = append(matchIDs, m.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return s.attachPlayersToMatches(ctx, matches, matchIDs)
}

// GetPlayerSessions returns recent sessions for a player (across all their GUIDs)
func (s *Store) GetPlayerSessions(ctx context.Context, playerID int64, limit int, beforeID *int64) ([]domain.PlayerSession, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := `
		SELECT s.id, s.server_id, srv.name, s.joined_at, s.left_at, s.duration_seconds, s.ip_address
		FROM sessions s
		JOIN player_guids pg ON s.player_guid_id = pg.id
		JOIN servers srv ON s.server_id = srv.id
		WHERE pg.player_id = ?`

	args := []interface{}{playerID}

	if beforeID != nil {
		query += ` AND s.id < ?`
		args = append(args, *beforeID)
	}

	query += ` ORDER BY s.joined_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []domain.PlayerSession
	for rows.Next() {
		var ps domain.PlayerSession
		var leftAt sql.NullTime
		var durationSeconds sql.NullInt64
		var ipAddress sql.NullString
		if err := rows.Scan(&ps.ID, &ps.ServerID, &ps.ServerName, &ps.JoinedAt, &leftAt, &durationSeconds, &ipAddress); err != nil {
			return nil, err
		}
		if leftAt.Valid {
			ps.LeftAt = &leftAt.Time
		}
		if durationSeconds.Valid {
			ps.DurationSeconds = durationSeconds.Int64
		}
		if ipAddress.Valid {
			ps.IPAddress = ipAddress.String
		}
		sessions = append(sessions, ps)
	}
	return sessions, rows.Err()
}
