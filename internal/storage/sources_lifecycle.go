package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Lifecycle errors, which handlers map to HTTP statuses.
var (
	ErrPendingRequestExists = errors.New("user already has a pending source request")
	ErrSourceNameTaken      = errors.New("source name is taken")
	ErrSourceNotPending     = errors.New("source is not in pending status")
	ErrSourceNotActive      = errors.New("source is not active")
)

// CreateSourceRequestArgs holds the fields a user fills in for a new
// source request. The collector's URL arrives later, in its
// registration heartbeats.
type CreateSourceRequestArgs struct {
	Source           string
	OwnerUserID      int64
	RequestedPurpose string
}

// CreateSourceRequest inserts a 'pending' source row owned by
// OwnerUserID. An owner may hold several active sources (one per
// host) but only one pending request, which keeps the admin queue
// short and stops a runaway client from queueing dozens.
//
// Three cases:
//
//  1. The caller already has a 'pending' row: ErrPendingRequestExists.
//
//  2. The caller owns a 'left' row with the requested name: reuse it,
//     approve it straight to 'active' (they were already trusted) and
//     refresh the purpose. The handler then mints new creds.
//
//  3. A new request: the name must not be taken by anyone else,
//     including a 'left' row, whose owner keeps the name. Insert a
//     new row.
func (s *Store) CreateSourceRequest(ctx context.Context, a CreateSourceRequestArgs) error {
	if err := ValidateSource(a.Source); err != nil {
		return fmt.Errorf("storage.CreateSourceRequest: %w", err)
	}
	if a.OwnerUserID == 0 {
		return fmt.Errorf("storage.CreateSourceRequest: owner_user_id is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. One pending request at a time.
	var existingStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT status FROM sources
		WHERE owner_user_id = ? AND status = 'pending'
		LIMIT 1
	`, a.OwnerUserID).Scan(&existingStatus)
	if err == nil {
		return ErrPendingRequestExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("storage.CreateSourceRequest: lookup own: %w", err)
	}

	// 2. Rejoin: same caller, same name, status='left'.
	var leftSource string
	err = tx.QueryRowContext(ctx, `
		SELECT source FROM sources
		WHERE owner_user_id = ? AND source = ? AND status = 'left'
	`, a.OwnerUserID, a.Source).Scan(&leftSource)
	if err == nil {
		if _, err := tx.ExecContext(ctx, `
			UPDATE sources
			SET status='active', active=1,
			    requested_purpose=?,
			    status_changed_at=CURRENT_TIMESTAMP
			WHERE source=?
		`, a.RequestedPurpose, a.Source); err != nil {
			return fmt.Errorf("storage.CreateSourceRequest: rejoin: %w", err)
		}
		return tx.Commit()
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("storage.CreateSourceRequest: lookup rejoin: %w", err)
	}

	// 3. New request: the name must be free.
	var taken int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sources
		WHERE source = ? AND status != 'left'
	`, a.Source).Scan(&taken); err != nil {
		return fmt.Errorf("storage.CreateSourceRequest: name check: %w", err)
	}
	if taken > 0 {
		return ErrSourceNameTaken
	}
	// Another user's 'left' row holds the name so its owner can rejoin.
	var leftByOther int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sources
		WHERE source = ? AND status = 'left' AND owner_user_id != ?
	`, a.Source, a.OwnerUserID).Scan(&leftByOther); err != nil {
		return fmt.Errorf("storage.CreateSourceRequest: left-by-other check: %w", err)
	}
	if leftByOther > 0 {
		return ErrSourceNameTaken
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sources
			(source, owner_user_id, status, active, is_remote,
			 requested_purpose, status_changed_at)
		VALUES (?, ?, 'pending', 0, 1, ?, CURRENT_TIMESTAMP)
	`, a.Source, a.OwnerUserID, a.RequestedPurpose); err != nil {
		return fmt.Errorf("storage.CreateSourceRequest: insert: %w", err)
	}
	return tx.Commit()
}

// scanFullSource decodes a row of sourceSelectColumns into a Source.
func scanFullSource(row scannable) (*Source, error) {
	var s Source
	var (
		hb        sql.NullTime
		changedAt sql.NullTime
		isRemote  int
		active    int
		reqPurp   sql.NullString
		rejReason sql.NullString
	)
	err := row.Scan(
		&s.Source, &s.DemoBaseURL, &s.Version, &hb, &isRemote, &active,
		&s.OwnerUserID, &s.Status, &reqPurp, &rejReason,
		&changedAt, &s.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if hb.Valid {
		s.LastHeartbeatAt = hb.Time
	}
	s.IsRemote = isRemote != 0
	s.Active = active != 0
	s.RequestedPurpose = reqPurp.String
	s.RejectionReason = rejReason.String
	s.StatusChangedAt = changedAt
	return &s, nil
}

// scannable is satisfied by both *sql.Row and *sql.Rows.
type scannable interface {
	Scan(dest ...any) error
}

const sourceSelectColumns = `
	source, demo_base_url, version, last_heartbeat_at, is_remote, active,
	owner_user_id, status, requested_purpose, rejection_reason,
	status_changed_at, created_at
`

// ListSourcesByOwner returns every source owned by ownerUserID: active
// rows first, then pending, then the rest newest first. The My servers
// drawer shows one card per row.
func (s *Store) ListSourcesByOwner(ctx context.Context, ownerUserID int64) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+sourceSelectColumns+`
		FROM sources
		WHERE owner_user_id = ?
		ORDER BY CASE status
			WHEN 'active'   THEN 0
			WHEN 'pending'  THEN 1
			WHEN 'left'     THEN 2
			WHEN 'rejected' THEN 3
			WHEN 'revoked'  THEN 4
			ELSE 5 END,
		COALESCE(status_changed_at, created_at) DESC
	`, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("storage.ListSourcesByOwner: %w", err)
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		src, err := scanFullSource(rows)
		if err != nil {
			return nil, fmt.Errorf("storage.ListSourcesByOwner: scan: %w", err)
		}
		out = append(out, *src)
	}
	return out, rows.Err()
}

// GetSourceByName returns the source row for a given name.
func (s *Store) GetSourceByName(ctx context.Context, source string) (*Source, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+sourceSelectColumns+`
		FROM sources WHERE source = ?
	`, source)
	src, err := scanFullSource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("storage.GetSourceByName: %w", err)
	}
	return src, nil
}

// ApproveSource moves a 'pending' row to 'active' (active=1). It
// returns ErrSourceNotPending if the row is missing or not pending.
func (s *Store) ApproveSource(ctx context.Context, source string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE sources
		SET status='active', active=1, status_changed_at=CURRENT_TIMESTAMP
		WHERE source = ? AND status = 'pending'
	`, source)
	if err != nil {
		return fmt.Errorf("storage.ApproveSource(%s): %w", source, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSourceNotPending
	}
	return nil
}

// RejectSource sets status='rejected' and stores the reason, which the
// API handler has already checked is non-empty.
func (s *Store) RejectSource(ctx context.Context, source, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("storage.RejectSource: reason is required")
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE sources
		SET status='rejected', active=0, rejection_reason=?,
		    status_changed_at=CURRENT_TIMESTAMP
		WHERE source = ? AND status = 'pending'
	`, reason, source)
	if err != nil {
		return fmt.Errorf("storage.RejectSource(%s): %w", source, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSourceNotPending
	}
	return nil
}

// LeaveSource sets status='left' (active=0) on an active or already
// left source and marks its servers inactive, as DeactivateSource does.
func (s *Store) LeaveSource(ctx context.Context, source string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		UPDATE sources
		SET status='left', active=0, status_changed_at=CURRENT_TIMESTAMP
		WHERE source = ? AND status IN ('active', 'left')
	`, source)
	if err != nil {
		return fmt.Errorf("storage.LeaveSource(%s): %w", source, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSourceNotActive
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE servers SET active = 0 WHERE source = ?
	`, source); err != nil {
		return fmt.Errorf("storage.LeaveSource(%s) servers: %w", source, err)
	}
	return tx.Commit()
}

// RenamePendingSource changes a pending source's name (an admin
// action). Only pending sources can be renamed, since no JWT is scoped
// to the old name and no collector is connected yet. It returns
// ErrSourceNotPending if the row isn't pending and ErrSourceNameTaken
// if newName collides with any non-'left' row. The new name must pass
// ValidateSource and the 3-character minimum the request form uses.
func (s *Store) RenamePendingSource(ctx context.Context, oldName, newName string) error {
	if err := ValidateSource(newName); err != nil {
		return fmt.Errorf("storage.RenamePendingSource: %w", err)
	}
	if len(newName) < 3 {
		return errors.New("storage.RenamePendingSource: name must be at least 3 characters")
	}
	if oldName == newName {
		return nil // no-op
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Foreign keys are enforced, and renaming sources.source briefly
	// orphans the source_audit rows, so the checks wait for COMMIT.
	if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
		return fmt.Errorf("storage.RenamePendingSource: defer fks: %w", err)
	}

	var status string
	if err := tx.QueryRowContext(ctx,
		"SELECT status FROM sources WHERE source = ?", oldName,
	).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSourceNotPending
		}
		return fmt.Errorf("storage.RenamePendingSource: lookup: %w", err)
	}
	if status != "pending" {
		return ErrSourceNotPending
	}

	var taken int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sources WHERE source = ? AND status != 'left'", newName,
	).Scan(&taken); err != nil {
		return fmt.Errorf("storage.RenamePendingSource: collision check: %w", err)
	}
	if taken > 0 {
		return ErrSourceNameTaken
	}

	// Every table that names the source follows the rename, so the
	// audit trail (at least the 'requested' row) moves with it.
	if _, err := tx.ExecContext(ctx,
		"UPDATE sources SET source = ? WHERE source = ?", newName, oldName,
	); err != nil {
		return fmt.Errorf("storage.RenamePendingSource: update sources: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE servers SET source = ? WHERE source = ?", newName, oldName,
	); err != nil {
		return fmt.Errorf("storage.RenamePendingSource: update servers: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE source_progress SET source = ? WHERE source = ?", newName, oldName,
	); err != nil {
		return fmt.Errorf("storage.RenamePendingSource: update source_progress: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE source_audit SET source = ? WHERE source = ?", newName, oldName,
	); err != nil {
		return fmt.Errorf("storage.RenamePendingSource: update source_audit: %w", err)
	}
	return tx.Commit()
}

// RevokeSourceStatus sets status='revoked', the admin's punitive state,
// which the owner can't rejoin from. Idempotent. DeactivateSource
// handles active and the servers.
func (s *Store) RevokeSourceStatus(ctx context.Context, source string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sources
		SET status='revoked', status_changed_at=CURRENT_TIMESTAMP
		WHERE source = ?
	`, source)
	if err != nil {
		return fmt.Errorf("storage.RevokeSourceStatus(%s): %w", source, err)
	}
	return nil
}

// PendingRequest is one row of the admin's pending list.
type PendingRequest struct {
	Source           string
	OwnerUserID      int64
	OwnerUsername    string
	RequestedPurpose string
	StatusChangedAt  time.Time
}

// ListPendingRequests returns every pending row with its owner's
// username, oldest first so admins review in submission order.
func (s *Store) ListPendingRequests(ctx context.Context) ([]PendingRequest, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.source, s.owner_user_id, u.username,
		       COALESCE(s.requested_purpose, ''),
		       s.status_changed_at
		FROM sources s
		JOIN users u ON u.id = s.owner_user_id
		WHERE s.status = 'pending'
		ORDER BY COALESCE(s.status_changed_at, s.created_at) ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("storage.ListPendingRequests: %w", err)
	}
	defer rows.Close()
	var out []PendingRequest
	for rows.Next() {
		var (
			p         PendingRequest
			changedAt sql.NullTime
		)
		if err := rows.Scan(&p.Source, &p.OwnerUserID, &p.OwnerUsername,
			&p.RequestedPurpose, &changedAt); err != nil {
			return nil, fmt.Errorf("storage.ListPendingRequests: scan: %w", err)
		}
		if changedAt.Valid {
			p.StatusChangedAt = changedAt.Time
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SourceServer is the servers row view the owner's My servers drawer
// shows.
type SourceServer struct {
	Key     string
	Address string
	Active  bool
}

// ListServersForSource returns one source's servers, empty until its
// collector first registers.
func (s *Store) ListServersForSource(ctx context.Context, source string) ([]SourceServer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT key, address, active
		FROM servers
		WHERE source = ?
		ORDER BY COALESCE(local_id, id)
	`, source)
	if err != nil {
		return nil, fmt.Errorf("storage.ListServersForSource(%s): %w", source, err)
	}
	defer rows.Close()
	var out []SourceServer
	for rows.Next() {
		var (
			ss     SourceServer
			active int
		)
		if err := rows.Scan(&ss.Key, &ss.Address, &active); err != nil {
			return nil, err
		}
		ss.Active = active != 0
		out = append(out, ss)
	}
	return out, rows.Err()
}

// SourceOwners maps source to owner_user_id for every source with an
// owner (the hub's local source has none), so the live-server endpoint
// computes manageable_by_me in one query.
func (s *Store) SourceOwners(ctx context.Context) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT source, owner_user_id FROM sources WHERE owner_user_id IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("storage.SourceOwners: %w", err)
	}
	defer rows.Close()
	out := make(map[string]int64)
	for rows.Next() {
		var src string
		var uid int64
		if err := rows.Scan(&src, &uid); err != nil {
			return nil, err
		}
		out[src] = uid
	}
	return out, rows.Err()
}
