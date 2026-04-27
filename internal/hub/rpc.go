package hub

import (
	"context"
	"time"
)

// RPCClient is the call-site contract for the collector's
// greet/claim/link flows. In standalone mode *Writer satisfies it
// directly; in distributed mode it is satisfied by a NATS req/reply
// wrapper. Routing is chosen by main.go.
type RPCClient interface {
	Greet(ctx context.Context, req GreetRequest) (GreetReply, error)
	Claim(ctx context.Context, req ClaimRequest) (ClaimReply, error)
	Link(ctx context.Context, req LinkRequest) (LinkReply, error)
}

// RPC request/reply types for synchronous flows between the collector
// and the hub writer. In standalone mode these travel through in-process
// channels; in distributed mode they are published as NATS request/reply
// on `trinity.rpc.<kind>.<source_id>`. The shapes are identical across
// transports.
//
// The collector holds the rcon connection and owns all player-facing
// output. The hub holds the DB and owns all identity resolution, stats
// lookups, and claim/link bookkeeping.

// AuthResult enumerates the outcomes of a Trinity auth attempt folded
// into the greet RPC.
type AuthResult string

const (
	AuthVerified        AuthResult = "verified"
	AuthFailed          AuthResult = "failed"
	AuthUnauthenticated AuthResult = "unauthenticated"
)

// GreetRequest is sent once per player join. The collector assembles it
// from its in-memory client state plus (optionally) the Trinity
// handshake parse. Auth is nil when the client did not attempt a
// handshake; the hub treats it as unauthenticated.
type GreetRequest struct {
	ServerID      int64      `json:"server_id"`
	MatchUUID     string     `json:"match_uuid,omitempty"`
	GUID          string     `json:"guid"`
	ClientName    string     `json:"client_name"`
	CleanName     string     `json:"clean_name"`
	ClientEngine  string     `json:"client_engine,omitempty"`
	ClientVersion string     `json:"client_version,omitempty"`
	Auth          *AuthProof `json:"auth,omitempty"`
}

// AuthProof carries a Trinity auth handshake. The hub looks up
// `users.game_token` for Username and verifies TokenHash ==
// sipHashHex(token, Nonce).
type AuthProof struct {
	Username  string `json:"username"`
	Nonce     string `json:"nonce"`
	TokenHash string `json:"token_hash"`
}

// GreetReply tells the collector what welcome to print and gives it the
// player_id / verified flags needed for downstream event population.
type GreetReply struct {
	AuthResult       AuthResult `json:"auth_result"`
	PlayerID         int64      `json:"player_id"`
	CanonicalName    string     `json:"canonical_name"`
	Claimed          bool       `json:"claimed"`
	IsVerified       bool       `json:"is_verified"`
	IsAdmin          bool       `json:"is_admin"`
	GUIDLinked       bool       `json:"guid_linked"` // set if this greet just merged the GUID onto an authed user's player
	KDRatio          float64    `json:"kd_ratio"`
	CompletedMatches int64      `json:"completed_matches"`
}

// ClaimStatus enumerates the outcomes of a `!claim` chat command.
type ClaimStatus string

const (
	ClaimOK             ClaimStatus = "ok"
	ClaimAlreadyClaimed ClaimStatus = "already_claimed"
	ClaimUnknownPlayer  ClaimStatus = "unknown_player"
	ClaimError          ClaimStatus = "error"
)

// ClaimRequest is sent when a player issues `!claim` in chat. The
// collector forwards the player's current GUID and its best guess at
// PlayerID (from the most recent greet reply).
type ClaimRequest struct {
	GUID     string `json:"guid"`
	PlayerID int64  `json:"player_id"`
}

// ClaimReply carries the generated code (when Status == ClaimOK) or the
// reason for failure. The collector prints the code to the player.
type ClaimReply struct {
	Status    ClaimStatus `json:"status"`
	Code      string      `json:"code,omitempty"`
	ExpiresAt time.Time   `json:"expires_at,omitempty"`
	Message   string      `json:"message,omitempty"`
}

// LinkStatus enumerates the outcomes of a `!link <code>` chat command.
type LinkStatus string

const (
	LinkOK            LinkStatus = "ok"
	LinkInvalidFormat LinkStatus = "invalid_format"
	LinkInvalidCode   LinkStatus = "invalid_code"
	LinkAlreadyLinked LinkStatus = "already_linked"
	LinkUnknownGUID   LinkStatus = "unknown_guid"
	LinkError         LinkStatus = "error"
)

// LinkRequest is sent when a player issues `!link <code>` in chat.
type LinkRequest struct {
	GUID string `json:"guid"`
	Code string `json:"code"`
}

// LinkReply tells the collector the outcome and (on success) the new
// PlayerID so it can update its in-memory client state.
type LinkReply struct {
	Status      LinkStatus `json:"status"`
	NewPlayerID int64      `json:"new_player_id,omitempty"`
	Message     string     `json:"message,omitempty"`
}
