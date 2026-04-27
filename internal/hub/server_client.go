package hub

import (
	"context"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// ServerClient is the call-site contract for collector → hub operations
// that were originally direct DB reads/writes on *Writer. In standalone
// mode *Writer satisfies it directly; in distributed mode it is
// satisfied by a NATS request/reply wrapper (natsbus.RPCClient).
//
// The split mirrors RPCClient: the collector depends on the narrow
// interface, main.go picks the implementation by config.
type ServerClient interface {
	RegisterServer(ctx context.Context, name, address, logPath string) (*domain.Server, error)

	UpsertPlayerIdentity(ctx context.Context, guid, name, cleanName string, ts time.Time, isVR bool) (PlayerIdentity, error)
	UpsertBotPlayerIdentity(ctx context.Context, name, cleanName string, ts time.Time) (PlayerIdentity, error)
	LookupPlayerIdentity(ctx context.Context, guid string) (PlayerIdentity, error)
}

// RegisterServerRequest / Reply carry the RegisterServer RPC payload.
// In distributed mode the hub upserts a servers row (creating it if new
// for a pending source) and tags it with (source_uuid, local_id) so
// event envelopes resolve correctly even before admin approval.
type RegisterServerRequest struct {
	SourceUUID string `json:"source_uuid"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	LogPath    string `json:"log_path"`
}

type RegisterServerReply struct {
	Server *domain.Server `json:"server,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// Identity RPC payloads.
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

