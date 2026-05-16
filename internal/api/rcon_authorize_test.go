package api

import (
	"testing"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/natsbus"
)

func TestAuthorizeUserForRcon(t *testing.T) {
	const (
		ownerID = int64(7)
		adminID = int64(42)
		otherID = int64(99)
	)
	owners := map[string]int64{"alice-q3": ownerID}

	tests := []struct {
		name        string
		userID      int64
		isAdmin     bool
		server      *domain.Server
		localSource string
		wantRole    natsbus.RconRole
		wantOk      bool
	}{
		{
			name:     "owner of remote source",
			userID:   ownerID,
			server:   &domain.Server{Source: "alice-q3"},
			wantRole: natsbus.RconRoleOwner,
			wantOk:   true,
		},
		{
			// User-added collector: owner is not is_admin, but still
			// gets RconRoleOwner via the source ownership arm.
			name:     "non-admin owner of their own source",
			userID:   ownerID,
			isAdmin:  false,
			server:   &domain.Server{Source: "alice-q3", AdminDelegationEnabled: false},
			wantRole: natsbus.RconRoleOwner,
			wantOk:   true,
		},
		{
			name:        "admin on local source short-circuits to owner",
			userID:      adminID,
			isAdmin:     true,
			server:      &domain.Server{Source: "hub-q3"},
			localSource: "hub-q3",
			wantRole:    natsbus.RconRoleOwner,
			wantOk:      true,
		},
		{
			name:     "admin on remote source without delegation denied",
			userID:   adminID,
			isAdmin:  true,
			server:   &domain.Server{Source: "alice-q3", AdminDelegationEnabled: false},
			wantRole: "",
			wantOk:   false,
		},
		{
			name:     "admin on remote source with delegation gets hub_admin",
			userID:   adminID,
			isAdmin:  true,
			server:   &domain.Server{Source: "alice-q3", AdminDelegationEnabled: true},
			wantRole: natsbus.RconRoleHubAdmin,
			wantOk:   true,
		},
		{
			name:     "random user denied",
			userID:   otherID,
			server:   &domain.Server{Source: "alice-q3", AdminDelegationEnabled: true},
			wantRole: "",
			wantOk:   false,
		},
		{
			name:     "owner takes precedence over hub_admin even if also admin",
			userID:   ownerID,
			isAdmin:  true,
			server:   &domain.Server{Source: "alice-q3", AdminDelegationEnabled: true},
			wantRole: natsbus.RconRoleOwner,
			wantOk:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, ok := AuthorizeUserForRcon(tt.userID, tt.isAdmin, tt.server, tt.localSource, owners)
			if role != tt.wantRole || ok != tt.wantOk {
				t.Errorf("got (%q, %v); want (%q, %v)", role, ok, tt.wantRole, tt.wantOk)
			}
		})
	}
}
