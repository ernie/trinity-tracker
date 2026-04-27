package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// PendingSource mirrors one row of pending_sources for admin workflows.
type PendingSource struct {
	SourceUUID  string
	Source      string
	FirstSeen   time.Time
	LastSeen    time.Time
	Version     string
	ServersJSON string
}

// UpsertPendingSource inserts or refreshes a pending_sources row,
// capturing the roster as JSON for the admin UI to display. first_seen
// is set on insert (default CURRENT_TIMESTAMP); last_seen, version, and
// servers_json refresh on every heartbeat while the source is pending.
func (s *Store) UpsertPendingSource(ctx context.Context, reg domain.Registration) error {
	roster, err := json.Marshal(reg.Servers)
	if err != nil {
		return fmt.Errorf("storage: marshal roster: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO pending_sources (source_uuid, source, last_seen, version, servers_json)
		VALUES (?, ?, CURRENT_TIMESTAMP, ?, ?)
		ON CONFLICT (source_uuid) DO UPDATE SET
			source       = excluded.source,
			last_seen    = excluded.last_seen,
			version      = excluded.version,
			servers_json = excluded.servers_json
	`, reg.SourceUUID, reg.Source, reg.Version, string(roster))
	if err != nil {
		return fmt.Errorf("storage: UpsertPendingSource(%s): %w", reg.SourceUUID, err)
	}
	return nil
}

// DeletePendingSource removes the pending row. Called on approve or
// reject.
func (s *Store) DeletePendingSource(ctx context.Context, sourceUUID string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM pending_sources WHERE source_uuid = ?", sourceUUID)
	if err != nil {
		return fmt.Errorf("storage: DeletePendingSource(%s): %w", sourceUUID, err)
	}
	return nil
}

// ListPendingSources returns all pending sources ordered by first_seen
// ascending (oldest first). For the admin UI in phase 11.
func (s *Store) ListPendingSources(ctx context.Context) ([]PendingSource, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT source_uuid, source, first_seen, last_seen, COALESCE(version, ''), COALESCE(servers_json, '[]')
		FROM pending_sources ORDER BY first_seen ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("storage: ListPendingSources: %w", err)
	}
	defer rows.Close()

	var out []PendingSource
	for rows.Next() {
		var p PendingSource
		var last sql.NullTime
		if err := rows.Scan(&p.SourceUUID, &p.Source, &p.FirstSeen, &last, &p.Version, &p.ServersJSON); err != nil {
			return nil, err
		}
		if last.Valid {
			p.LastSeen = last.Time
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// IsSourceApproved returns true if any servers row references this
// source_uuid. The registration handler uses this to decide between
// updating last_heartbeat_at (approved) and upserting a pending row
// (unknown).
func (s *Store) IsSourceApproved(ctx context.Context, sourceUUID string) (bool, error) {
	var dummy int
	err := s.db.QueryRowContext(ctx,
		"SELECT 1 FROM servers WHERE source_uuid = ? LIMIT 1", sourceUUID,
	).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("storage: IsSourceApproved(%s): %w", sourceUUID, err)
	}
	return true, nil
}

// GetServerLastHeartbeat returns the stored last_heartbeat_at for a
// servers row. Returns the zero time if the column is NULL.
func (s *Store) GetServerLastHeartbeat(ctx context.Context, serverID int64) (time.Time, error) {
	var t sql.NullTime
	err := s.db.QueryRowContext(ctx, "SELECT last_heartbeat_at FROM servers WHERE id = ?", serverID).Scan(&t)
	if err != nil {
		return time.Time{}, err
	}
	if !t.Valid {
		return time.Time{}, nil
	}
	return t.Time, nil
}

// TouchSourceHeartbeat updates last_heartbeat_at on every servers row
// belonging to the source_uuid. No-op if no rows match.
func (s *Store) TouchSourceHeartbeat(ctx context.Context, sourceUUID string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE servers SET last_heartbeat_at = ? WHERE source_uuid = ?",
		formatTimestamp(at), sourceUUID,
	)
	if err != nil {
		return fmt.Errorf("storage: TouchSourceHeartbeat(%s): %w", sourceUUID, err)
	}
	return nil
}

// TagLocalServerSource attaches a source_uuid and local_id to an
// existing servers row, marking it as approved for that source. Used
// by main.go to wire up a local collector's source_uuid to the pre-
// existing local servers (hub+collector deployment).
func (s *Store) TagLocalServerSource(ctx context.Context, serverID int64, sourceUUID string, localID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE servers SET source_uuid = ?, local_id = ? WHERE id = ?
	`, sourceUUID, localID, serverID)
	if err != nil {
		return fmt.Errorf("storage: TagLocalServerSource(%d): %w", serverID, err)
	}
	return nil
}

// ResolveServerIDForSource maps (source_uuid, remote_local_id) to the
// hub's own servers.id. Returns 0 and no error if the mapping does
// not exist (source is pending or unknown). The subscriber uses this
// to translate Envelope.RemoteServerID before dispatch.
func (s *Store) ResolveServerIDForSource(ctx context.Context, sourceUUID string, localID int64) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		"SELECT id FROM servers WHERE source_uuid = ? AND local_id = ? LIMIT 1",
		sourceUUID, localID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("storage: ResolveServerIDForSource: %w", err)
	}
	return id, nil
}

// RemoteServer is a lightweight view of a servers row tagged
// is_remote=1. Used by the hub's poller to loop over remote targets.
type RemoteServer struct {
	ID            int64
	Name          string
	RemoteAddress string
	SourceUUID    string
}

// LookupSourceIDByUUID returns the human-readable source name of the
// first servers row whose source_uuid matches. Empty string if none.
// Used by the creds-rotation admin endpoint to build new permissions
// scoped to the collector's actual source_id.
func (s *Store) LookupSourceIDByUUID(ctx context.Context, sourceUUID string) (string, error) {
	if sourceUUID == "" {
		return "", nil
	}
	var source string
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(source, '') FROM servers
		WHERE source_uuid = ? LIMIT 1
	`, sourceUUID).Scan(&source)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("storage: LookupSourceIDByUUID: %w", err)
	}
	return source, nil
}

// ListPollableServers returns every servers row the hub poller should
// UDP-poll: all rows with a non-empty address (COALESCE remote_address,
// address). Covers both is_remote=0 rows (local collectors or
// standalone) and is_remote=1 rows (remote collectors). Ordered by id.
func (s *Store) ListPollableServers(ctx context.Context) ([]RemoteServer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name,
		       COALESCE(NULLIF(remote_address, ''), COALESCE(address, '')),
		       COALESCE(source_uuid, '')
		FROM servers
		WHERE COALESCE(NULLIF(remote_address, ''), COALESCE(address, '')) <> ''
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("storage: ListPollableServers: %w", err)
	}
	defer rows.Close()
	var out []RemoteServer
	for rows.Next() {
		var r RemoteServer
		if err := rows.Scan(&r.ID, &r.Name, &r.RemoteAddress, &r.SourceUUID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRemoteServers returns all servers rows where is_remote=1 and
// remote_address is populated. Ordered by id.
func (s *Store) ListRemoteServers(ctx context.Context) ([]RemoteServer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, COALESCE(remote_address, ''), COALESCE(source_uuid, '')
		FROM servers
		WHERE is_remote = 1 AND COALESCE(remote_address, '') <> ''
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("storage: ListRemoteServers: %w", err)
	}
	defer rows.Close()
	var out []RemoteServer
	for rows.Next() {
		var r RemoteServer
		if err := rows.Scan(&r.ID, &r.Name, &r.RemoteAddress, &r.SourceUUID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ApproveRemoteServers creates (or updates) servers rows for every
// entry in the registration's roster, stamping them is_remote=1 and
// attaching source_uuid/local_id. Idempotent: repeated approvals of
// the same source refresh the rows. Address uniqueness is not
// enforced when is_remote=1, since two remote collectors may publish
// the same address (different networks); we use (source_uuid, local_id)
// as the logical key.
func (s *Store) ApproveRemoteServers(ctx context.Context, reg domain.Registration, demoBaseURL string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, rs := range reg.Servers {
		// If a row already exists for (source_uuid, local_id), update it.
		var existingID int64
		err := tx.QueryRowContext(ctx,
			"SELECT id FROM servers WHERE source_uuid = ? AND local_id = ? LIMIT 1",
			reg.SourceUUID, rs.LocalID,
		).Scan(&existingID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO servers (name, address, source, source_uuid, local_id, remote_address, is_remote, last_heartbeat_at, demo_base_url)
				VALUES (?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, ?)
			`, rs.Name, rs.Address, reg.Source, reg.SourceUUID, rs.LocalID, rs.Address, demoBaseURL); err != nil {
				return fmt.Errorf("storage: insert remote server: %w", err)
			}
		case err != nil:
			return err
		default:
			if _, err := tx.ExecContext(ctx, `
				UPDATE servers SET name = ?, address = ?, source = ?, remote_address = ?, last_heartbeat_at = CURRENT_TIMESTAMP, demo_base_url = ?
				WHERE id = ?
			`, rs.Name, rs.Address, reg.Source, rs.Address, demoBaseURL, existingID); err != nil {
				return fmt.Errorf("storage: update remote server: %w", err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM pending_sources WHERE source_uuid = ?", reg.SourceUUID); err != nil {
		return fmt.Errorf("storage: clear pending row: %w", err)
	}
	return tx.Commit()
}
