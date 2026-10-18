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

// sourceIDPattern keeps source names safe as a NATS subject token, a
// URL path segment and a filename (the .creds file lives on the hub's
// disk).
var sourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ValidateSource returns an error if s is not a valid source name. It
// runs before provisioning so a bad name never reaches creds files,
// NATS permissions or URL routing.
func ValidateSource(s string) error {
	if s == "" {
		return fmt.Errorf("source must be non-empty")
	}
	if len(s) > 16 {
		return fmt.Errorf("source must be at most 16 chars")
	}
	if !sourceIDPattern.MatchString(s) {
		return fmt.Errorf("source %q must match %s", s, sourceIDPattern.String())
	}
	return nil
}

// Source is one row of the sources table, the hub's record of a
// collector. The hub accepts a collector's messages only when a row
// here names its source.
type Source struct {
	Source           string
	DemoBaseURL      string
	Version          string
	LastHeartbeatAt  time.Time
	IsRemote         bool
	Active           bool
	OwnerUserID      sql.NullInt64
	Status           string
	RequestedPurpose string
	RejectionReason  string
	StatusChangedAt  sql.NullTime
	CreatedAt        time.Time
}

// CreateSource inserts a new sources row for a remote collector (admin
// flow) or the hub's own local collector (startup). It errors on a
// duplicate source. demo_base_url starts empty; the collector's
// registration heartbeat fills it in, so the operator can change it
// without an admin.
//
// ownerUserID is required for remote sources, so RCON delegation,
// rotate and leave have a user to authorize. Pass nil for the hub's
// local collector, which belongs to whoever runs the box.
func (s *Store) CreateSource(ctx context.Context, source string, isRemote bool, ownerUserID *int64) error {
	if err := ValidateSource(source); err != nil {
		return fmt.Errorf("storage.CreateSource: %w", err)
	}
	if isRemote && ownerUserID == nil {
		return fmt.Errorf("storage.CreateSource: remote sources require an owner_user_id")
	}
	remoteInt := 0
	if isRemote {
		remoteInt = 1
	}
	var owner sql.NullInt64
	if ownerUserID != nil {
		owner = sql.NullInt64{Int64: *ownerUserID, Valid: true}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sources (source, is_remote, owner_user_id)
		VALUES (?, ?, ?)
	`, source, remoteInt, owner)
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

// DeactivateSource marks a source and all of its servers inactive. The
// rows stay so past matches and sessions keep their (source, key)
// reference, and the UI dims them. The admin endpoint pairs this with
// creds revocation.
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

// ReactivateSource sets the source active again, status included, and
// reactivates all of its servers. A server the operator removed from
// cfg.Q3Servers goes inactive again when the collector next starts.
func (s *Store) ReactivateSource(ctx context.Context, source string) error {
	if source == "" {
		return fmt.Errorf("storage.ReactivateSource: source is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE sources SET active = 1, status = 'active', status_changed_at = CURRENT_TIMESTAMP
		WHERE source = ?`, source); err != nil {
		return fmt.Errorf("storage: ReactivateSource sources(%s): %w", source, err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE servers SET active = 1 WHERE source = ?", source); err != nil {
		return fmt.Errorf("storage: ReactivateSource servers(%s): %w", source, err)
	}
	return tx.Commit()
}

// IsSourceApproved reports whether the source has an active row in the
// sources table. Event ingest and RPC handlers check it as a second
// line behind NATS auth, which blocks unknown or deactivated sources at
// the broker.
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

// TouchSourceHeartbeat updates last_heartbeat_at, version and the
// operator-owned demo_base_url on the sources row and every servers row
// for the source. An empty version leaves the stored one; demoBaseURL
// always wins, since the operator's config owns it, so an empty value
// clears it.
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
// servers row, wiring a combined hub and collector's local servers to
// its source.
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
// most recent, or "" when none has. The asset-fallback handlers
// (/demopk3s/maps, /assets/levelshots) use it for files the hub lacks.
// The map match ignores case: the UI lowercases URLs, but
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
// whose server hosted the match identified by uuid, or "" when there's
// no such match.
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

// FindSourcePublicURL returns the demo_base_url of the named source, or
// "" if the source is unknown or has none set. The hub's live relay
// router uses it to redirect a viewer to the collector that owns the
// server.
func (s *Store) FindSourcePublicURL(ctx context.Context, source string) (string, error) {
	var url string
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(demo_base_url, '')
		FROM sources
		WHERE source = ?
	`, source).Scan(&url)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("storage: FindSourcePublicURL(%q): %w", source, err)
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

// RemoteServer is a lightweight view of a servers row for the hub's
// poller and directory gate. IsRemote and UserPubKey come from the
// source's row; UserPubKey identifies the source's live NATS
// connection and stays empty until its creds have been minted.
type RemoteServer struct {
	ID         int64
	Source     string
	Key        string
	Address    string
	IsRemote   bool
	UserPubKey string
}

// ListPollableServers returns the servers rows the hub poller UDP-polls,
// ordered by id: active rows, local or remote, with an address, that
// have been seen enforcing g_trinityHandshake. Only those show in live
// status.
func (s *Store) ListPollableServers(ctx context.Context) ([]RemoteServer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.source, s.key, s.address, src.is_remote, src.user_pubkey
		FROM servers s
		JOIN sources src ON src.source = s.source
		WHERE s.active = 1 AND s.handshake_required = 1 AND s.address <> ''
		ORDER BY s.id
	`)
	if err != nil {
		return nil, fmt.Errorf("storage: ListPollableServers: %w", err)
	}
	defer rows.Close()
	var out []RemoteServer
	for rows.Next() {
		var r RemoteServer
		if err := rows.Scan(&r.ID, &r.Source, &r.Key, &r.Address, &r.IsRemote, &r.UserPubKey); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListDirectoryGateEntries returns active servers rows with an address,
// the membership gate for the Q3 directory server. A server registers
// with the directory before its first match, so this gate ignores
// handshake_required.
func (s *Store) ListDirectoryGateEntries(ctx context.Context) ([]RemoteServer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.source, s.key, s.address, src.is_remote, src.user_pubkey
		FROM servers s
		JOIN sources src ON src.source = s.source
		WHERE s.active = 1 AND s.address <> ''
		ORDER BY s.id
	`)
	if err != nil {
		return nil, fmt.Errorf("storage: ListDirectoryGateEntries: %w", err)
	}
	defer rows.Close()
	var out []RemoteServer
	for rows.Next() {
		var r RemoteServer
		if err := rows.Scan(&r.ID, &r.Source, &r.Key, &r.Address, &r.IsRemote, &r.UserPubKey); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRemoteServers returns every servers row whose source is remote.
// Ordered by id.
func (s *Store) ListRemoteServers(ctx context.Context) ([]RemoteServer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.source, s.key, s.address
		FROM servers s
		JOIN sources src ON src.source = s.source
		WHERE src.is_remote = 1 AND s.address <> ''
		ORDER BY s.id
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
		r.IsRemote = true
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetSourceUserPubKey returns the NATS user pubkey stored for source,
// or "" when none has been minted or the source row doesn't exist.
// natsbus.AuthStore's mint and revoke use it.
func (s *Store) GetSourceUserPubKey(ctx context.Context, source string) (string, error) {
	var pub string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_pubkey FROM sources WHERE source = ?`, source,
	).Scan(&pub)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("storage: GetSourceUserPubKey: %w", err)
	}
	return pub, nil
}

// SetSourceUserPubKey writes the NATS user pubkey for source, for
// AuthStore.MintUserCreds. It errors if source has no row in sources.
func (s *Store) SetSourceUserPubKey(ctx context.Context, source, pubkey string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sources SET user_pubkey = ? WHERE source = ?`,
		pubkey, source,
	)
	if err != nil {
		return fmt.Errorf("storage: SetSourceUserPubKey: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("storage: SetSourceUserPubKey: source %q not found", source)
	}
	return nil
}

// ApprovedSource is the per-source data the admin UI shows: the source
// name, the version and time of its latest heartbeat (for the health
// color) and its servers. IsRemote=false means the hub's own local
// collector, which has no .creds file.
type ApprovedSource struct {
	Source          string
	Version         string
	LastHeartbeatAt time.Time
	DemoBaseURL     string
	IsRemote        bool
	Active          bool
	OwnerUserID     sql.NullInt64
	OwnerUsername   string
	Servers         []ApprovedSourceServer
}

// ApprovedSourceServer is one of an approved source's servers rows.
type ApprovedSourceServer struct {
	ID      int64
	LocalID int64
	Key     string
	Address string
	Active  bool
}

// ListApprovedSources returns every sources row with its servers,
// ordered by source. A source that hasn't registered yet has an empty
// Servers slice.
func (s *Store) ListApprovedSources(ctx context.Context) ([]ApprovedSource, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			s.source,
			s.version,
			s.demo_base_url,
			s.last_heartbeat_at,
			s.is_remote,
			s.active,
			s.owner_user_id,
			COALESCE(u.username, ''),
			sv.id,
			COALESCE(sv.local_id, 0),
			COALESCE(sv.key, ''),
			COALESCE(sv.address, ''),
			COALESCE(sv.active, 0)
		FROM sources s
		LEFT JOIN users u ON u.id = s.owner_user_id
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
			source        string
			version       string
			demoBaseURL   string
			hb            sql.NullTime
			isRemote      int
			sourceActive  int
			ownerUserID   sql.NullInt64
			ownerUsername string
			srvID         sql.NullInt64
			localID       int64
			key           string
			address       string
			srvActive     int
		)
		if err := rows.Scan(&source, &version, &demoBaseURL, &hb, &isRemote, &sourceActive, &ownerUserID, &ownerUsername, &srvID, &localID, &key, &address, &srvActive); err != nil {
			return nil, err
		}
		entry, ok := bySource[source]
		if !ok {
			entry = &ApprovedSource{
				Source:        source,
				Version:       version,
				DemoBaseURL:   demoBaseURL,
				IsRemote:      isRemote != 0,
				Active:        sourceActive != 0,
				OwnerUserID:   ownerUserID,
				OwnerUsername: ownerUsername,
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

// TransferSourceOwner reassigns owner_user_id for a remote source. It
// errors for a local source, which has no owner, and for a missing
// source. The caller checks that the new owner exists; the foreign key
// checks again at write time.
func (s *Store) TransferSourceOwner(ctx context.Context, source string, newOwnerUserID int64) error {
	if source == "" {
		return fmt.Errorf("storage.TransferSourceOwner: source is required")
	}
	if newOwnerUserID <= 0 {
		return fmt.Errorf("storage.TransferSourceOwner: owner_user_id is required")
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE sources
		   SET owner_user_id = ?
		 WHERE source = ? AND is_remote = 1
	`, newOwnerUserID, source)
	if err != nil {
		return fmt.Errorf("storage.TransferSourceOwner(%s): %w", source, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("storage.TransferSourceOwner: source %q not found or not remote", source)
	}
	return nil
}

// UpsertRemoteServers creates or refreshes a servers row, with source
// and local_id, for every entry in the registration roster. The
// registration handler calls it on each heartbeat, since the roster is
// the collector's server list. demo_base_url comes from the heartbeat,
// matching what TouchSourceHeartbeat wrote to the sources row.
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
		delegate := 0
		if rs.AdminDelegationEnabled {
			delegate = 1
		}
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO servers (key, address, source, local_id, active, last_heartbeat_at, demo_base_url, source_version, admin_delegation_enabled)
				VALUES (?, ?, ?, ?, 1, CURRENT_TIMESTAMP, ?, ?, ?)
			`, rs.Key, rs.Address, reg.Source, rs.LocalID, reg.DemoBaseURL, reg.Version, delegate); err != nil {
				return fmt.Errorf("storage: insert remote server: %w", err)
			}
		case err != nil:
			return err
		default:
			if _, err := tx.ExecContext(ctx, `
				UPDATE servers SET key = ?, address = ?, active = 1, last_heartbeat_at = CURRENT_TIMESTAMP, demo_base_url = ?, source_version = ?, admin_delegation_enabled = ?
				WHERE id = ?
			`, rs.Key, rs.Address, reg.DemoBaseURL, reg.Version, delegate, existingID); err != nil {
				return fmt.Errorf("storage: update remote server: %w", err)
			}
		}
	}
	// A server missing from the roster was removed from cfg.Q3Servers.
	if err := deactivateMissing(ctx, tx, reg.Source, keptKeys); err != nil {
		return err
	}
	return tx.Commit()
}

// deactivateMissing sets active=0 on every active server of source
// whose key isn't in keep, ignoring case.
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
