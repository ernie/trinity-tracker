package hub

import (
	"context"
	"time"
)

// RPCClient is the collector → hub contract for greet/claim/link.
type RPCClient interface {
	Greet(ctx context.Context, req GreetRequest) (GreetReply, error)
	Claim(ctx context.Context, req ClaimRequest) (ClaimReply, error)
	Link(ctx context.Context, req LinkRequest) (LinkReply, error)
}

// AuthResult reports whether the optional auth info inside a Trinity
// handshake validated this session. Independent of GreetReply.IsVerified,
// which reflects whether the GUID is already linked to a user account.
type AuthResult string

const (
	AuthVerified        AuthResult = "verified"        // auth info present and validated
	AuthFailed          AuthResult = "failed"          // auth info present but invalid
	AuthUnauthenticated AuthResult = "unauthenticated" // no auth info sent
)

// GreetRequest is sent once per player join. Auth is nil when the
// client did not attempt a Trinity handshake.
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

// AuthProof is the optional trailing portion of the Trinity handshake
// that proves account ownership via SipHash: TokenHash ==
// sipHashHex(users.game_token[Username], Nonce). When present and
// valid, the hub auto-links the GUID to the named user's player. A
// player can still be verified (GUID already linked) without sending
// AuthProof.
type AuthProof struct {
	Username  string `json:"username"`
	Nonce     string `json:"nonce"`
	TokenHash string `json:"token_hash"`
}

// GreetReply: IsVerified means the GUID is linked to a user account
// (green-checkmark state). GUIDLinked means this greet just did the
// linking. AuthResult reports session-scoped auth outcome.
type GreetReply struct {
	AuthResult       AuthResult `json:"auth_result"`
	CanonicalName    string     `json:"canonical_name"`
	Claimed          bool       `json:"claimed"`
	IsVerified       bool       `json:"is_verified"`
	IsAdmin          bool       `json:"is_admin"`
	GUIDLinked       bool       `json:"guid_linked"`
	KDRatio          float64    `json:"kd_ratio"`
	CompletedMatches int64      `json:"completed_matches"`
}

type ClaimStatus string

const (
	ClaimOK             ClaimStatus = "ok"
	ClaimAlreadyClaimed ClaimStatus = "already_claimed"
	ClaimUnknownPlayer  ClaimStatus = "unknown_player"
	ClaimError          ClaimStatus = "error"
)

type ClaimRequest struct {
	GUID string `json:"guid"`
}

type ClaimReply struct {
	Status    ClaimStatus `json:"status"`
	Code      string      `json:"code,omitempty"`
	ExpiresAt time.Time   `json:"expires_at,omitempty"`
	Message   string      `json:"message,omitempty"`
}

type LinkStatus string

const (
	LinkOK            LinkStatus = "ok"
	LinkInvalidFormat LinkStatus = "invalid_format"
	LinkInvalidCode   LinkStatus = "invalid_code"
	LinkAlreadyLinked LinkStatus = "already_linked"
	LinkUnknownGUID   LinkStatus = "unknown_guid"
	LinkError         LinkStatus = "error"
)

type LinkRequest struct {
	GUID string `json:"guid"`
	Code string `json:"code"`
}

type LinkReply struct {
	Status  LinkStatus `json:"status"`
	Message string     `json:"message,omitempty"`
}
