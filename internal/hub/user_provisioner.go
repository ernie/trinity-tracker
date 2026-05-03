package hub

import "context"

// UserProvisioner mints, revokes, and locates per-source NATS credentials.
type UserProvisioner interface {
	// MintUserCreds issues (or re-issues, revoking the prior) user JWT
	// and returns the .creds file contents.
	MintUserCreds(ctx context.Context, source string) ([]byte, error)
	RevokeSource(ctx context.Context, source string) error
	CredsPath(source string) string
}
