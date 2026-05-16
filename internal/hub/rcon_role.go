package hub

import "github.com/ernie/trinity-tracker/internal/domain"

// RconRole is what the hub asserts about the calling user. The
// collector trusts the hub on identity (JWT or handshake-auth was
// already validated) but re-checks RconRoleHubAdmin against its
// per-server allow_hub_admin_rcon delegation flag.
//
// Lives in the hub package so both the request-reply path
// (internal/natsbus) and the policy decision in this file can be
// reused without an import cycle through internal/api. The natsbus
// package re-exports the same symbol via type alias for backward
// compatibility with collectors that already speak the wire format.
type RconRole string

const (
	RconRoleOwner    RconRole = "owner"
	RconRoleHubAdmin RconRole = "hub_admin"
)

// AuthorizeUserForRcon is the rcon-access policy: given a user (by id
// + admin bit) and a server, what role should the collector honor?
// Pure function — caller supplies sourceOwners (from store.SourceOwners)
// so it works in both HTTP and handshake contexts without re-querying.
//
// The collector re-validates RconRoleHubAdmin against its local
// AllowHubAdminRcon cfg before the rcon password is touched.
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
