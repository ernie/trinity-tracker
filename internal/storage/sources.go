package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// sourceIDPattern validates admin-chosen source identifiers. The rule
// is conservative: alphanumerics, underscore, hyphen — enough to be
// human-readable while staying safe as a NATS subject token, as a URL
// path segment, and as a filename (.creds lives on the hub's disk).
var sourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ValidateSource returns an error if s is not a syntactically valid
// source identifier. Used at provisioning time so a typo can't wedge
// creds files, NATS permissions, or URL routing after the fact.
func ValidateSource(s string) error {
	if s == "" {
		return fmt.Errorf("source must be non-empty")
	}
	if len(s) > 64 {
		return fmt.Errorf("source must be at most 64 chars")
	}
	if !sourceIDPattern.MatchString(s) {
		return fmt.Errorf("source %q must match %s", s, sourceIDPattern.String())
	}
	return nil
}

// Source is one row of the sources table — the hub's record of a
// pre-provisioned collector. A collector may only publish anything
// the hub accepts if a row here names its source.
type Source struct {
	Source          string
	DemoBaseURL     string
	Version         string
	LastHeartbeatAt time.Time
	IsRemote        bool
	CreatedAt       time.Time
}

// CreateSource inserts a new sources row. Called by the admin flow
// (remote collectors) and by main.go at startup for the hub's own
// local collector. Errors on duplicate source. demo_base_url is
// left empty — the operator populates it via registration heartbeat
// so they can change it without admin intervention.
func (s *Store) CreateSource(ctx context.Context, source string, isRemote bool) error {
	if err := ValidateSource(source); err != nil {
		return fmt.Errorf("storage.CreateSource: %w", err)
	}
	remoteInt := 0
	if isRemote {
		remoteInt = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sources (source, is_remote)
		VALUES (?, ?)
	`, source, remoteInt)
	if err != nil {
		return fmt.Errorf("storage: CreateSource(%s): %w", source, err)
	}
	return nil
}

// UpsertLocalSource is a narrow form of CreateSource for the hub's own
// local collector: idempotent on restart, so a repeated boot over the
// same DB reuses the row instead of erroring.
func (s *Store) UpsertLocalSource(ctx context.Context, source string) error {
	if err := ValidateSource(source); err != nil {
		return fmt.Errorf("storage.UpsertLocalSource: %w", err)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sources (source, is_remote)
		VALUES (?, 0)
		ON CONFLICT (source) DO UPDATE SET is_remote = 0
	`, source)
	if err != nil {
		return fmt.Errorf("storage: UpsertLocalSource(%s): %w", source, err)
	}
	return nil
}

// DeactivateSource marks a source inactive and cascades to all of its
// servers. Rows are kept so historical matches/sessions retain their
// (source, key) reference; the UI dims inactive content rather than
// dropping it. The admin endpoint pairs this with creds revocation.
func (s *Store) DeactivateSource(ctx context.Context, source string) error {
	if source == "" {
		return fmt.Errorf("storage.DeactivateSource: source is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE servers SET active = 0 WHERE source = ?", source); err != nil {
		return fmt.Errorf("storage: DeactivateSource servers(%s): %w", source, err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE sources SET active = 0 WHERE source = ?", source); err != nil {
		return fmt.Errorf("storage: DeactivateSource sources(%s): %w", source, err)
	}
	return tx.Commit()
}

// ReactivateSource is the inverse: flip the source row back on and
// cascade to its servers. Individual per-server activation state is
// not preserved across a deactivate/reactivate cycle — operators who
// want a server to stay decommissioned after reactivating its source
// should leave it out of cfg.Q3Servers (which the collector startup
// loop will then mark inactive again).
func (s *Store) ReactivateSource(ctx context.Context, source string) error {
	if source == "" {
		return fmt.Errorf("storage.ReactivateSource: source is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE sources SET active = 1 WHERE source = ?", source); err != nil {
		return fmt.Errorf("storage: ReactivateSource sources(%s): %w", source, err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE servers SET active = 1 WHERE source = ?", source); err != nil {
		return fmt.Errorf("storage: ReactivateSource servers(%s): %w", source, err)
	}
	return tx.Commit()
}

// IsSourceApproved returns true if the source has a row in the sources
// table AND is active. Event ingest and RPC handlers use this as a
// defense-in-depth check; NATS auth should already have blocked
// unknown or deactivated sources at the broker.
func (s *Store) IsSourceApproved(ctx context.Context, source string) (bool, error) {
	var dummy int
	err := s.db.QueryRowContext(ctx,
		"SELECT 1 FROM sources WHERE source = ? AND active = 1 LIMIT 1", source,
	).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("storage: IsSourceApproved(%s): %w", source, err)
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

// TouchSourceHeartbeat updates last_heartbeat_at, version, and the
// operator-owned demo_base_url on the sources row + every servers row
// for the source. Version is only overwritten when non-empty;
// demoBaseURL is authoritative (operator config is the source of
// truth), so an empty value clears whatever was there.
func (s *Store) TouchSourceHeartbeat(ctx context.Context, source string, at time.Time, version, demoBaseURL string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stamp := formatTimestamp(at)
	if _, err := tx.ExecContext(ctx,
		"UPDATE sources SET last_heartbeat_at = ?, version = COALESCE(NULLIF(?, ''), version), demo_base_url = ? WHERE source = ?",
		stamp, version, demoBaseURL, source,
	); err != nil {
		return fmt.Errorf("storage: TouchSourceHeartbeat sources(%s): %w", source, err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE servers SET last_heartbeat_at = ?, source_version = COALESCE(NULLIF(?, ''), source_version), demo_base_url = ? WHERE source = ?",
		stamp, version, demoBaseURL, source,
	); err != nil {
		return fmt.Errorf("storage: TouchSourceHeartbeat servers(%s): %w", source, err)
	}
	return tx.Commit()
}

// TagLocalServerSource attaches a source and local_id to an existing
// servers row. Used by main.go to wire up a local collector's source
// to the pre-existing local servers (hub+collector deployment).
func (s *Store) TagLocalServerSource(ctx context.Context, serverID int64, source string, localID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE servers SET source = ?, local_id = ? WHERE id = ?
	`, source, localID, serverID)
	if err != nil {
		return fmt.Errorf("storage: TagLocalServerSource(%d): %w", serverID, err)
	}
	return nil
}

// FindSourcePublicURLForMap returns the demo_base_url of any source
// whose servers have hosted a match on the named map, preferring the
// most recent. Empty string + nil error if none found. Used by the
// asset-fallback handlers (/demopk3s/maps, /assets/levelshots) when
// the hub doesn't have the file locally. Map name match is
// case-insensitive: incoming URLs are downcased by the UI, but
// matches.map_name is stored as the engine reports it.
func (s *Store) FindSourcePublicURLForMap(ctx context.Context, mapName string) (string, error) {
	var url string
	err := s.db.QueryRowContext(ctx, `
		SELECT s.demo_base_url
		FROM matches m
		JOIN servers sv ON sv.id = m.server_id
		JOIN sources s  ON s.source = sv.source
		WHERE LOWER(m.map_name) = LOWER(?) AND COALESCE(s.demo_base_url, '') <> ''
		ORDER BY m.id DESC
		LIMIT 1
	`, mapName).Scan(&url)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("storage: FindSourcePublicURLForMap(%q): %w", mapName, err)
	}
	return url, nil
}

// FindSourcePublicURLForDemo returns the demo_base_url of the source
// whose server hosted the match identified by uuid. Empty string +
// nil error if none found.
func (s *Store) FindSourcePublicURLForDemo(ctx context.Context, uuid string) (string, error) {
	var url string
	err := s.db.QueryRowContext(ctx, `
		SELECT s.demo_base_url
		FROM matches m
		JOIN servers sv ON sv.id = m.server_id
		JOIN sources s  ON s.source = sv.source
		WHERE m.uuid = ? AND COALESCE(s.demo_base_url, '') <> ''
	`, uuid).Scan(&url)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("storage: FindSourcePublicURLForDemo(%q): %w", uuid, err)
	}
	return url, nil
}

// ResolveServerIDForSource maps (source, remote_local_id) to the hub's
// own servers.id. Returns 0 and no error if no row matches. The
// subscriber uses this to translate Envelope.RemoteServerID before
// dispatch.
func (s *Store) ResolveServerIDForSource(ctx context.Context, source string, localID int64) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		"SELECT id FROM servers WHERE source = ? AND local_id = ? LIMIT 1",
		source, localID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("storage: ResolveServerIDForSource: %w", err)
	}
	return id, nil
}

// RemoteServer is a lightweight view of a servers row, used by the
// hub's poller to iterate pollable targets.
type RemoteServer struct {
	ID      int64
	Source  string
	Key     string
	Address string
}

// ListPollableServers returns every servers row the hub poller should
// UDP-poll: active rows with a non-empty address. Covers both
// is_remote=0 rows (the hub's local collector) and is_remote=1 rows
// (remote collectors). Inactive rows are skipped — they're
// decommissioned and shouldn't show up in live status. Ordered by id.
func (s *Store) ListPollableServers(ctx context.Context) ([]RemoteServer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source, key, address
		FROM servers
		WHERE active = 1 AND address <> ''
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("storage: ListPollableServers: %w", err)
	}
	defer rows.Close()
	var out []RemoteServer
	for rows.Next() {
		var r RemoteServer
		if err := rows.Scan(&r.ID, &r.Source, &r.Key, &r.Address); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRemoteServers returns all servers rows where is_remote=1.
// Ordered by id.
func (s *Store) ListRemoteServers(ctx context.Context) ([]RemoteServer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source, key, address
		FROM servers
		WHERE is_remote = 1 AND address <> ''
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("storage: ListRemoteServers: %w", err)
	}
	defer rows.Close()
	var out []RemoteServer
	for rows.Next() {
		var r RemoteServer
		if err := rows.Scan(&r.ID, &r.Source, &r.Key, &r.Address); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ApprovedSource groups the per-source metadata the admin UI needs:
// the source identifier + version pulled from the latest heartbeat,
// the timestamp of that heartbeat (so the UI can color-code health),
// and the rollup of servers owned by this source. IsRemote=false
// means the hub's own local collector — no per-source .creds file
// is ever minted for it.
type ApprovedSource struct {
	Source          string
	Version         string
	LastHeartbeatAt time.Time
	DemoBaseURL     string
	IsRemote        bool
	Active          bool
	Servers         []ApprovedSourceServer
}

// ApprovedSourceServer is one servers row attached to an approved
// source — id, key, and the address the hub poller uses.
type ApprovedSourceServer struct {
	ID      int64
	LocalID int64
	Key     string
	Address string
	Active  bool
}

// ListApprovedSources returns every row from the sources table with
// its attached servers (if any). A newly created source with no
// registration yet shows up with an empty Servers slice. Ordered by
// source for stable UI rendering.
func (s *Store) ListApprovedSources(ctx context.Context) ([]ApprovedSource, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			s.source,
			s.version,
			s.demo_base_url,
			s.last_heartbeat_at,
			s.is_remote,
			s.active,
			sv.id,
			COALESCE(sv.local_id, 0),
			COALESCE(sv.key, ''),
			COALESCE(sv.address, ''),
			COALESCE(sv.active, 0)
		FROM sources s
		LEFT JOIN servers sv ON sv.source = s.source
		ORDER BY s.source, COALESCE(sv.local_id, sv.id)
	`)
	if err != nil {
		return nil, fmt.Errorf("storage: ListApprovedSources: %w", err)
	}
	defer rows.Close()

	bySource := make(map[string]*ApprovedSource)
	var order []string
	for rows.Next() {
		var (
			source       string
			version      string
			demoBaseURL  string
			hb           sql.NullTime
			isRemote     int
			sourceActive int
			srvID        sql.NullInt64
			localID      int64
			key          string
			address      string
			srvActive    int
		)
		if err := rows.Scan(&source, &version, &demoBaseURL, &hb, &isRemote, &sourceActive, &srvID, &localID, &key, &address, &srvActive); err != nil {
			return nil, err
		}
		entry, ok := bySource[source]
		if !ok {
			entry = &ApprovedSource{
				Source:      source,
				Version:     version,
				DemoBaseURL: demoBaseURL,
				IsRemote:    isRemote != 0,
				Active:      sourceActive != 0,
			}
			if hb.Valid {
				entry.LastHeartbeatAt = hb.Time
			}
			bySource[source] = entry
			order = append(order, source)
		}
		if srvID.Valid {
			entry.Servers = append(entry.Servers, ApprovedSourceServer{
				ID:      srvID.Int64,
				LocalID: localID,
				Key:     key,
				Address: address,
				Active:  srvActive != 0,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]ApprovedSource, 0, len(order))
	for _, source := range order {
		out = append(out, *bySource[source])
	}
	return out, nil
}

// UpsertRemoteServers creates or refreshes servers rows for every
// entry in the registration roster, stamping them is_remote=1 and
// attaching source/local_id. Called from the registration handler on
// each heartbeat — rosters are the collector's source of truth for
// the server list. demo_base_url comes straight from the heartbeat
// payload; TouchSourceHeartbeat already wrote it to sources earlier
// in the same handler, so this keeps the two rows in lockstep.
func (s *Store) UpsertRemoteServers(ctx context.Context, reg domain.Registration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	keptKeys := make([]string, 0, len(reg.Servers))
	for _, rs := range reg.Servers {
		keptKeys = append(keptKeys, rs.Key)
		var existingID int64
		err := tx.QueryRowContext(ctx,
			"SELECT id FROM servers WHERE source = ? AND local_id = ? LIMIT 1",
			reg.Source, rs.LocalID,
		).Scan(&existingID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO servers (key, address, source, local_id, is_remote, active, last_heartbeat_at, demo_base_url, source_version)
				VALUES (?, ?, ?, ?, 1, 1, CURRENT_TIMESTAMP, ?, ?)
			`, rs.Key, rs.Address, reg.Source, rs.LocalID, reg.DemoBaseURL, reg.Version); err != nil {
				return fmt.Errorf("storage: insert remote server: %w", err)
			}
		case err != nil:
			return err
		default:
			if _, err := tx.ExecContext(ctx, `
				UPDATE servers SET key = ?, address = ?, active = 1, last_heartbeat_at = CURRENT_TIMESTAMP, demo_base_url = ?, source_version = ?
				WHERE id = ?
			`, rs.Key, rs.Address, reg.DemoBaseURL, reg.Version, existingID); err != nil {
				return fmt.Errorf("storage: update remote server: %w", err)
			}
		}
	}
	// Anything tied to this source that wasn't in the heartbeat roster
	// — operator dropped it from cfg.Q3Servers — is now inactive.
	if err := deactivateMissing(ctx, tx, reg.Source, keptKeys); err != nil {
		return err
	}
	return tx.Commit()
}

// deactivateMissing flips active=0 on every active server tied to
// `source` whose key isn't in `keep` (case-insensitive). Used inside
// the UpsertRemoteServers transaction.
func deactivateMissing(ctx context.Context, tx *sql.Tx, source string, keep []string) error {
	if len(keep) == 0 {
		_, err := tx.ExecContext(ctx,
			"UPDATE servers SET active = 0 WHERE source = ? AND active = 1", source)
		if err != nil {
			return fmt.Errorf("storage: deactivateMissing(%s): %w", source, err)
		}
		return nil
	}
	placeholders := make([]string, len(keep))
	args := []interface{}{source}
	for i, k := range keep {
		placeholders[i] = "?"
		args = append(args, k)
	}
	q := "UPDATE servers SET active = 0 WHERE source = ? AND active = 1 AND key NOT IN (" +
		strings.Join(placeholders, ",") + ") COLLATE NOCASE"
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("storage: deactivateMissing(%s): %w", source, err)
	}
	return nil
}
