package hub

import "github.com/ernie/trinity-tracker/internal/domain"

// RconRole is the role the hub asserts for the calling user. The
// collector trusts the hub on identity but rechecks RconRoleHubAdmin
// against its per-server allow_hub_admin_rcon flag. It lives in hub so
// natsbus and the policy below share it without an import cycle
// through internal/api.
type RconRole string

const (
	RconRoleOwner    RconRole = "owner"
	RconRoleHubAdmin RconRole = "hub_admin"
)

// AuthorizeUserForRcon returns the rcon role, if any, the collector
// should honor for the user on server. The caller passes sourceOwners
// (from store.SourceOwners) so it works in HTTP and handshake contexts
// without querying again.
func AuthorizeUserForRcon(userID int64, isAdmin bool, server *domain.Server, localSource string, sourceOwners map[string]int64) (RconRole, bool) {
	ownerID, hasOwner := sourceOwners[server.Source]
	isLocal := localSource != "" && server.Source == localSource
	switch {
	case hasOwner && ownerID == userID:
		return RconRoleOwner, true
	case isAdmin && isLocal:
		return RconRoleOwner, true
	case isAdmin && server.AdminDelegationEnabled:
		return RconRoleHubAdmin, true
	default:
		return "", false
	}
}
