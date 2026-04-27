package hub

// UserProvisioner is the admin-flow hook for minting, revoking, and
// locating per-source NATS credentials. Implemented by
// natsbus.AuthStore. The hub package declares the interface so the API
// router can depend on it without pulling in natsbus.
type UserProvisioner interface {
	// MintUserCreds issues (or re-issues, revoking the old) a user
	// JWT for sourceID/sourceUUID, persists a .creds file, and
	// returns its contents.
	MintUserCreds(sourceID, sourceUUID string) ([]byte, error)

	// RevokeSource revokes whatever user is currently active for
	// sourceUUID. No-op if nothing is recorded.
	RevokeSource(sourceUUID string) error

	// CredsPath returns the on-disk path to the source's .creds file.
	CredsPath(sourceUUID string) string
}
