package storage

import (
	"context"
	"database/sql"
	"errors"
)

// ErrPATNotFound is returned when no live PAT matches a hash — the
// token is unknown, revoked, expired, or its user was deleted.
var ErrPATNotFound = errors.New("storage: pat not found")

// PATIdentity is what a valid PAT resolves to: the token row's id
// (for last-used touches) plus the *live* user row, so claims built
// from it reflect current privileges, not mint-time ones.
type PATIdentity struct {
	PATID                  int64
	UserID                 int64
	Username               string
	IsAdmin                bool
	PlayerID               *int64
	PasswordChangeRequired bool
}

// CreatePAT inserts a personal access token for a user. tokenHash is
// the at-rest form (auth.HashPAT); the plaintext never reaches storage.
func (s *Store) CreatePAT(ctx context.Context, userID int64, name, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pat (user_id, name, token_hash) VALUES (?, ?, ?)
	`, userID, name, tokenHash)
	return err
}

// LookupPATByHash resolves a presented token hash to a live identity.
// Revoked and expired tokens (and tokens whose user is gone) yield
// ErrPATNotFound — indistinguishable from an unknown token on purpose.
func (s *Store) LookupPATByHash(ctx context.Context, tokenHash string) (*PATIdentity, error) {
	var id PATIdentity
	var playerID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT p.id, u.id, u.username, u.is_admin, u.player_id, u.password_change_required
		FROM pat p JOIN users u ON u.id = p.user_id
		WHERE p.token_hash = ?
		  AND p.revoked_at IS NULL
		  AND (p.expires_at IS NULL OR p.expires_at > CURRENT_TIMESTAMP)
	`, tokenHash).Scan(&id.PATID, &id.UserID, &id.Username, &id.IsAdmin, &playerID, &id.PasswordChangeRequired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPATNotFound
	}
	if err != nil {
		return nil, err
	}
	if playerID.Valid {
		id.PlayerID = &playerID.Int64
	}
	return &id, nil
}

// RevokePATByHash revokes the token with the given hash. Returns
// ErrPATNotFound if no live token matched (already revoked or unknown)
// so callers can stay idempotent without a second query.
func (s *Store) RevokePATByHash(ctx context.Context, tokenHash string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE pat SET revoked_at = CURRENT_TIMESTAMP
		WHERE token_hash = ? AND revoked_at IS NULL
	`, tokenHash)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPATNotFound
	}
	return nil
}

// TouchPATLastUsed records token activity. Callers throttle; this is
// a plain unconditional write.
func (s *Store) TouchPATLastUsed(ctx context.Context, patID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE pat SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?
	`, patID)
	return err
}
