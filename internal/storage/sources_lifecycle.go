package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Lifecycle errors. Handlers map these to specific HTTP statuses
// (typically 409 Conflict for taken/duplicate, 404 for not-pending).
var (
	ErrPendingRequestExists = errors.New("user already has a pending source request")
	ErrSourceNameTaken      = errors.New("source name is taken")
	ErrSourceNotPending     = errors.New("source is not in pending status")
	ErrSourceNotActive      = errors.New("source is not active")
)

// CreateSourceRequestArgs captures the user-facing fields of a new
// source request. The collector's reachable URL is reported via
// registration heartbeats — no need for the operator to type it in.
type CreateSourceRequestArgs struct {
	Source           string
	OwnerUserID      int64
	RequestedPurpose string
}

// CreateSourceRequest inserts a 'pending' source row owned by
// OwnerUserID. The owner may hold multiple actives concurrently
// (one-source-per-host deployment pattern), but only one *pending*
// request at a time — keeps the admin queue tidy and prevents a
// runaway loop from queueing dozens.
//
// Three branching cases:
//
//  1. Caller already has a 'pending' row → ErrPendingRequestExists.
//
//  2. Caller owns a 'left' row with the requested name → reuse it,
//     auto-approve to 'active' (they were already trusted), refresh
//     the purpose. The handler then re-mints creds.
//
//  3. Fresh request: name must not be taken by anyone else (any
//     status except 'left'+different owner — left rows still hold
//     ownership of the name). Insert a new row.
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

	// 1. Block second pending while one is already in flight. Multiple
	// 'active' rows are fine — one source per host is the intended
	// deployment pattern.
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

	// 2. Rejoin path: same caller, same name, status='left' → reuse.
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

	// 3. Fresh request: name must be free (left rows still claim it).
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
	// A 'left' row owned by a *different* user blocks the name too:
	// the original owner can come back even after walking away.
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

// scanFullSource decodes a row carrying every column of the modern
// sources table into a Source struct. Used by ListSourcesByOwner and
// GetSourceByName.
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

// scannable abstracts *sql.Row vs *sql.Rows so the same scanner is
// reused for single-row and multi-row queries.
type scannable interface {
	Scan(dest ...any) error
}

const sourceSelectColumns = `
	source, demo_base_url, version, last_heartbeat_at, is_remote, active,
	owner_user_id, status, requested_purpose, rejection_reason,
	status_changed_at, created_at
`

// ListSourcesByOwner returns every source owned by ownerUserID,
// ordered with active rows first, then pending, then the rest by
// recency. Used by GET /api/sources/mine to drive the My Servers
// drawer (which renders one card per source).
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

// GetSourceByName returns the source row for a given name. Used by
// admin handlers (approve/reject) and by handlers that already know
// the source token (rotate/leave) to look up status + ownership.
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

// ApproveSource flips a 'pending' row to 'active' (active=1). Returns
// ErrSourceNotPending if the row doesn't exist or isn't pending — the
// admin endpoint maps that to 404/409.
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

// RejectSource sets status='rejected' and stores a non-empty reason.
// Empty reason is a programming error (the API handler validates).
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

// LeaveSource sets status='left' (active=0). Allowed from 'active'
// only (idempotent on already-left). Cascades server rows inactive
// to mirror DeactivateSource's behavior.
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

// RenamePendingSource changes a pending source's name (admin
// privilege). Allowed only while status='pending' so we don't have
// to deal with a minted JWT scoped to the old name or a connected
// collector. Returns ErrSourceNotPending if the row isn't pending,
// ErrSourceNameTaken if newName collides with any non-'left' row.
//
// The new name must pass ValidateSource AND the user-facing 3-32
// length rule (the request modal enforces this on entry; admins
// renaming should respect the same convention).
func (s *Store) RenamePendingSource(ctx context.Context, oldName, newName string) error {
	if err := ValidateSource(newName); err != nil {
		return fmt.Errorf("storage.RenamePendingSource: %w", err)
	}
	if len(newName) < 3 || len(newName) > 32 {
		return errors.New("storage.RenamePendingSource: name must be 3-32 characters")
	}
	if oldName == newName {
		return nil // no-op
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// FK enforcement is on (PRAGMA foreign_keys=ON in sqlite.go);
	// updating sources.source would transiently orphan source_audit
	// rows pointing at the old name. Defer FK checks to COMMIT so the
	// parent + children update in one consistent shot.
	if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
		return fmt.Errorf("storage.RenamePendingSource: defer fks: %w", err)
	}

	// Confirm the row is pending.
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

	// New name must not collide with any non-left source.
	var taken int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sources WHERE source = ? AND status != 'left'", newName,
	).Scan(&taken); err != nil {
		return fmt.Errorf("storage.RenamePendingSource: collision check: %w", err)
	}
	if taken > 0 {
		return ErrSourceNameTaken
	}

	// Update the row + every child table that references source by
	// name. Pending rows shouldn't have servers/source_progress yet,
	// but the audit table will have at least the 'requested' entry —
	// keep the trail under the new name and write a 'renamed' marker.
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

// RevokeSourceStatus sets status='revoked' (admin punitive). Idempotent.
// The cascade to servers is handled by the existing DeactivateSource
// call path; this only updates the status column for audit/UI clarity.
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

// PendingRequest is the admin-pending-list row: source name, owner,
// what they asked for, and when they submitted.
type PendingRequest struct {
	Source           string
	OwnerUserID      int64
	OwnerUsername    string
	RequestedPurpose string
	StatusChangedAt  time.Time
}

// ListPendingRequests returns every status='pending' row, joined to
// users to expose the owner's username for the admin UI. Oldest-first
// so admins triage in submission order.
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

// SourceServer is a lightweight view of a servers row used by the
// owner-side My Servers drawer. Mirrors ApprovedSourceServer but
// trimmed of fields the drawer doesn't render.
type SourceServer struct {
	Key     string
	Address string
	Active  bool
}

// ListServersForSource returns the server roster for one source.
// Empty slice if the source exists but has no registered servers yet
// (e.g. just-approved, collector hasn't connected).
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

// SourceOwners returns source -> owner_user_id for every source with a
// non-NULL owner. Local (admin-minted) sources have no owner row in
// this map. Used by the live-server endpoint to compute manageable_by_me
// without N+1 queries.
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
