package storage

import (
	"context"
	"database/sql"
	"fmt"
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
