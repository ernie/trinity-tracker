package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SourceAuditEntry is one row of source_audit. ActorUserID is null
// for system-driven actions (e.g. JWT auto-revoke on collector exit).
type SourceAuditEntry struct {
	ID          int64
	Source      string
	ActorUserID sql.NullInt64
	Action      string
	Detail      string
	CreatedAt   time.Time
}

// WriteSourceAudit records a lifecycle event. The handler layer calls
// this on every state change (requested/approved/rejected/rotated/
// downloaded/left/rejoined/revoked) so the trail survives restarts.
// Pass nil for actorUserID when the action is system-initiated.
func (s *Store) WriteSourceAudit(ctx context.Context, source string, actorUserID *int64, action, detail string) error {
	var actor sql.NullInt64
	if actorUserID != nil {
		actor = sql.NullInt64{Int64: *actorUserID, Valid: true}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO source_audit (source, actor_user_id, action, detail)
		VALUES (?, ?, ?, ?)
	`, source, actor, action, detail)
	if err != nil {
		return fmt.Errorf("storage.WriteSourceAudit(%s, %s): %w", source, action, err)
	}
	return nil
}

// AuditFilters narrows ListAllAudit results. Zero-value fields are
// treated as "no filter on this column."
type AuditFilters struct {
	Source string
	Actor  string // matches users.username
	Action string
	Since  time.Time
	Limit  int // <=0: default 100; capped at 500
}

// AuditEntryWithActor is a SourceAuditEntry joined to users.username
// so the admin UI can show "alice approved" without a second lookup.
// ActorUsername is empty for system-driven actions (ActorUserID null)
// or rows whose actor user has been deleted.
type AuditEntryWithActor struct {
	SourceAuditEntry
	ActorUsername string
}

// ListAllAudit returns audit rows across all sources, newest first,
// optionally filtered. Drives the global /admin/audit page.
func (s *Store) ListAllAudit(ctx context.Context, filters AuditFilters) ([]AuditEntryWithActor, error) {
	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	var conds []string
	var args []any
	if filters.Source != "" {
		conds = append(conds, "sa.source = ?")
		args = append(args, filters.Source)
	}
	if filters.Actor != "" {
		conds = append(conds, "u.username = ?")
		args = append(args, filters.Actor)
	}
	if filters.Action != "" {
		conds = append(conds, "sa.action = ?")
		args = append(args, filters.Action)
	}
	if !filters.Since.IsZero() {
		conds = append(conds, "sa.created_at >= ?")
		args = append(args, filters.Since)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, limit)

	q := fmt.Sprintf(`
		SELECT sa.id, sa.source, sa.actor_user_id, sa.action,
		       COALESCE(sa.detail, ''), sa.created_at,
		       COALESCE(u.username, '')
		FROM source_audit sa
		LEFT JOIN users u ON sa.actor_user_id = u.id
		%s
		ORDER BY sa.created_at DESC, sa.id DESC
		LIMIT ?
	`, where)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage.ListAllAudit: %w", err)
	}
	defer rows.Close()
	var out []AuditEntryWithActor
	for rows.Next() {
		var e AuditEntryWithActor
		if err := rows.Scan(
			&e.ID, &e.Source, &e.ActorUserID, &e.Action,
			&e.Detail, &e.CreatedAt,
			&e.ActorUsername,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListSourceAudit returns the most recent audit rows for a source,
// newest first. Limit caps the result; pass <=0 to get the default 50.
func (s *Store) ListSourceAudit(ctx context.Context, source string, limit int) ([]SourceAuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source, actor_user_id, action, COALESCE(detail, ''), created_at
		FROM source_audit
		WHERE source = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, source, limit)
	if err != nil {
		return nil, fmt.Errorf("storage.ListSourceAudit(%s): %w", source, err)
	}
	defer rows.Close()
	var out []SourceAuditEntry
	for rows.Next() {
		var e SourceAuditEntry
		if err := rows.Scan(&e.ID, &e.Source, &e.ActorUserID, &e.Action, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
