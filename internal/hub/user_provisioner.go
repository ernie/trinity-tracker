package hub

// UserProvisioner mints, revokes, and locates per-source NATS credentials.
type UserProvisioner interface {
	// MintUserCreds issues (or re-issues, revoking the prior) user JWT
	// and returns the .creds file contents.
	MintUserCreds(source string) ([]byte, error)
	RevokeSource(source string) error
	CredsPath(source string) string
}
