package storage

import (
	"context"
	"fmt"
	"time"
)

// DirectoryRegistration is one row of the persisted Q3 directory
// registry. The hub writes the full set at graceful shutdown and
// reads it back at startup; see internal/directory for the freshness
// gate that decides whether a snapshot is trustworthy.
type DirectoryRegistration struct {
	Addr        string
	ServerID    int64
	Protocol    int
	Gamename    string
	Engine      string
	Clients     int
	MaxClients  int
	Gametype    int
	ValidatedAt time.Time
	ExpiresAt   time.Time
}

// ListDirectoryRegistrations returns every persisted registration.
// Order is unspecified — the directory restores into a map keyed by
// addr, so order doesn't matter to the caller.
func (s *Store) ListDirectoryRegistrations(ctx context.Context) ([]DirectoryRegistration, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT addr, server_id, protocol, gamename, engine,
		       clients, max_clients, gametype, validated_at, expires_at
		FROM directory_registrations
	`)
	if err != nil {
		return nil, fmt.Errorf("storage: ListDirectoryRegistrations: %w", err)
	}
	defer rows.Close()
	var out []DirectoryRegistration
	for rows.Next() {
		var (
			r           DirectoryRegistration
			validatedAt int64
			expiresAt   int64
		)
		if err := rows.Scan(&r.Addr, &r.ServerID, &r.Protocol, &r.Gamename, &r.Engine,
			&r.Clients, &r.MaxClients, &r.Gametype, &validatedAt, &expiresAt); err != nil {
			return nil, err
		}
		r.ValidatedAt = time.Unix(validatedAt, 0).UTC()
		r.ExpiresAt = time.Unix(expiresAt, 0).UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReplaceDirectoryRegistrations atomically swaps the persisted set
// for the supplied rows: DELETE then bulk INSERT inside one
// transaction. Passing an empty slice leaves the table empty (same as
// ClearDirectoryRegistrations).
func (s *Store) ReplaceDirectoryRegistrations(ctx context.Context, rows []DirectoryRegistration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM directory_registrations"); err != nil {
		return fmt.Errorf("storage: ReplaceDirectoryRegistrations delete: %w", err)
	}
	if len(rows) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO directory_registrations
			  (addr, server_id, protocol, gamename, engine,
			   clients, max_clients, gametype, validated_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return fmt.Errorf("storage: ReplaceDirectoryRegistrations prepare: %w", err)
		}
		defer stmt.Close()
		for _, r := range rows {
			if _, err := stmt.ExecContext(ctx,
				r.Addr, r.ServerID, r.Protocol, r.Gamename, r.Engine,
				r.Clients, r.MaxClients, r.Gametype,
				r.ValidatedAt.Unix(), r.ExpiresAt.Unix(),
			); err != nil {
				return fmt.Errorf("storage: ReplaceDirectoryRegistrations insert %s: %w", r.Addr, err)
			}
		}
	}
	return tx.Commit()
}

// ClearDirectoryRegistrations empties the table. Called on startup
// when the persisted snapshot is older than the freshness window.
func (s *Store) ClearDirectoryRegistrations(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM directory_registrations"); err != nil {
		return fmt.Errorf("storage: ClearDirectoryRegistrations: %w", err)
	}
	return nil
}
