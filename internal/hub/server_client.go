package hub

import (
	"context"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// ServerClient is the collector → hub server/identity contract.
type ServerClient interface {
	RegisterServer(ctx context.Context, source, key, address string) (*domain.Server, error)

	UpsertPlayerIdentity(ctx context.Context, guid, name, cleanName string, ts time.Time, isVR bool) (PlayerIdentity, error)
	UpsertBotPlayerIdentity(ctx context.Context, name, cleanName string, ts time.Time) (PlayerIdentity, error)
	LookupPlayerIdentity(ctx context.Context, guid string) (PlayerIdentity, error)
}

// RegisterServerRequest / Reply carry the RegisterServer RPC payload.
// The hub tags the upserted row with (source, local_id=servers.id) so
// envelopes resolve.
type RegisterServerRequest struct {
	Source  string `json:"source"`
	Key     string `json:"key"`
	Address string `json:"address"`
}

type RegisterServerReply struct {
	Server *domain.Server `json:"server,omitempty"`
	Error  string         `json:"error,omitempty"`
}

type UpsertIdentityRequest struct {
	GUID      string    `json:"guid"`
	Name      string    `json:"name"`
	CleanName string    `json:"clean_name"`
	Timestamp time.Time `json:"ts"`
	IsVR      bool      `json:"is_vr"`
}

type UpsertBotIdentityRequest struct {
	Name      string    `json:"name"`
	CleanName string    `json:"clean_name"`
	Timestamp time.Time `json:"ts"`
}

type LookupIdentityRequest struct {
	GUID string `json:"guid"`
}

type IdentityReply struct {
	Identity PlayerIdentity `json:"identity"`
	Error    string         `json:"error,omitempty"`
}

