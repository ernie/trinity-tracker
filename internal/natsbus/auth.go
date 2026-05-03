package natsbus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nkeys"
)

// Directory layout under <hubDataDir>:
//   auth/{operator,operator_sign,sys,trinity,trinity_sign}.seed
//   auth/{operator,sys,trinity}.jwt (trinity.jwt rewritten on revoke)
//   auth/hub_internal.creds   in-process hub client creds
//   creds/<source>.creds      collector user creds (transport artifact)
//
// User pubkeys live in sources.user_pubkey, accessed via the
// PubKeyStore interface. Was previously creds/<source>.pub on disk;
// moved to the DB to consolidate per-source state on the source row
// and to decouple the directory/poller from this package's on-disk
// layout.

const (
	authSubdir  = "auth"
	credsSubdir = "creds"
	userPrefix  = "trinity"
)

// PubKeyStore is the slice of *storage.Store that AuthStore depends on
// for persisting per-source NATS user pubkeys. Defined here so the
// natsbus package owns its own dependency contract; *storage.Store
// satisfies it.
type PubKeyStore interface {
	GetSourceUserPubKey(ctx context.Context, source string) (string, error)
	SetSourceUserPubKey(ctx context.Context, source, pubkey string) error
}

// AuthStore owns the NKey/JWT material for the embedded NATS server
// and mints/revokes per-source user credentials.
type AuthStore struct {
	dir   string
	store PubKeyStore

	mu            sync.Mutex
	opKP          nkeys.KeyPair
	opSignKP      nkeys.KeyPair
	sysKP         nkeys.KeyPair
	trinityKP     nkeys.KeyPair
	trSignKP      nkeys.KeyPair
	opJWT         string
	sysJWT        string
	trinityJWT    string
	opClaims      *jwt.OperatorClaims
	resolver      *server.MemAccResolver
	runningSrv    *server.Server
	internalCreds []byte
}

// LoadOrCreateAuthStore initializes auth material under <hubDataDir>/auth/
// and prepares an account resolver with SYS + TRINITY JWTs. First run
// generates everything; subsequent runs load from disk. store is used
// by Mint/Revoke to persist per-source user pubkeys; pass nil only in
// tests that don't exercise those paths.
func LoadOrCreateAuthStore(hubDataDir string, store PubKeyStore) (*AuthStore, error) {
	s := &AuthStore{dir: hubDataDir, store: store}
	authDir := filepath.Join(hubDataDir, authSubdir)
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		return nil, fmt.Errorf("natsbus.auth: create %s: %w", authDir, err)
	}
	credsDir := filepath.Join(hubDataDir, credsSubdir)
	if err := os.MkdirAll(credsDir, 0o700); err != nil {
		return nil, fmt.Errorf("natsbus.auth: create %s: %w", credsDir, err)
	}

	var err error
	if s.opKP, err = loadOrCreateSeed(filepath.Join(authDir, "operator.seed"), nkeys.CreateOperator); err != nil {
		return nil, err
	}
	if s.opSignKP, err = loadOrCreateSeed(filepath.Join(authDir, "operator_sign.seed"), nkeys.CreateOperator); err != nil {
		return nil, err
	}
	if s.sysKP, err = loadOrCreateSeed(filepath.Join(authDir, "sys.seed"), nkeys.CreateAccount); err != nil {
		return nil, err
	}
	if s.trinityKP, err = loadOrCreateSeed(filepath.Join(authDir, "trinity.seed"), nkeys.CreateAccount); err != nil {
		return nil, err
	}
	if s.trSignKP, err = loadOrCreateSeed(filepath.Join(authDir, "trinity_sign.seed"), nkeys.CreateAccount); err != nil {
		return nil, err
	}

	if s.opJWT, err = loadOrCreateOperatorJWT(filepath.Join(authDir, "operator.jwt"), s.opKP, s.opSignKP, s.sysKP); err != nil {
		return nil, err
	}
	s.opClaims, err = jwt.DecodeOperatorClaims(s.opJWT)
	if err != nil {
		return nil, fmt.Errorf("natsbus.auth: decode operator JWT: %w", err)
	}
	if s.sysJWT, err = loadOrCreateAccountJWT(filepath.Join(authDir, "sys.jwt"), s.sysKP, s.opSignKP, "SYS", nil); err != nil {
		return nil, err
	}
	trSignPub, _ := s.trSignKP.PublicKey()
	if s.trinityJWT, err = loadOrCreateAccountJWT(filepath.Join(authDir, "trinity.jwt"), s.trinityKP, s.opSignKP, "TRINITY", []string{trSignPub}); err != nil {
		return nil, err
	}

	s.resolver = &server.MemAccResolver{}
	sysPub, _ := s.sysKP.PublicKey()
	trPub, _ := s.trinityKP.PublicKey()
	if err := s.resolver.Store(sysPub, s.sysJWT); err != nil {
		return nil, fmt.Errorf("natsbus.auth: resolver store SYS: %w", err)
	}
	if err := s.resolver.Store(trPub, s.trinityJWT); err != nil {
		return nil, fmt.Errorf("natsbus.auth: resolver store TRINITY: %w", err)
	}

	// Rewritten every boot so regeneration is automatic after any seed loss.
	if s.internalCreds, err = s.buildInternalCreds(filepath.Join(authDir, "hub_internal.creds")); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *AuthStore) TrustedOperators() []*jwt.OperatorClaims {
	return []*jwt.OperatorClaims{s.opClaims}
}

func (s *AuthStore) SystemAccountPublicKey() string {
	pk, _ := s.sysKP.PublicKey()
	return pk
}

func (s *AuthStore) Resolver() *server.MemAccResolver {
	return s.resolver
}

// AttachServer records a running server so Mint/Revoke can push live
// account-claim updates. Call once after natsbus.Start.
func (s *AuthStore) AttachServer(ns *server.Server) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runningSrv = ns
}

func (s *AuthStore) InternalCredsPath() string {
	return filepath.Join(s.dir, authSubdir, "hub_internal.creds")
}

func (s *AuthStore) CredsPath(source string) string {
	return filepath.Join(s.dir, credsSubdir, source+".creds")
}

// MintUserCreds issues a per-source user JWT with subject-scoped
// publish permissions and persists the .creds file. Records the new
// user pubkey on the sources row; revokes the prior pubkey if one was
// recorded. Returns the creds file contents (the operator copies the
// .creds file to the collector machine).
func (s *AuthStore) MintUserCreds(ctx context.Context, source string) ([]byte, error) {
	if s.store == nil {
		return nil, fmt.Errorf("natsbus.auth: MintUserCreds requires a PubKeyStore")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	oldPub, err := s.store.GetSourceUserPubKey(ctx, source)
	if err != nil {
		return nil, err
	}

	userKP, err := nkeys.CreateUser()
	if err != nil {
		return nil, fmt.Errorf("natsbus.auth: create user kp: %w", err)
	}
	userPub, _ := userKP.PublicKey()
	userSeed, _ := userKP.Seed()

	uc := jwt.NewUserClaims(userPub)
	uc.Name = userPrefix + "-" + source
	trPub, _ := s.trinityKP.PublicKey()
	uc.IssuerAccount = trPub
	addSourcePermissions(uc, source)

	userJWT, err := uc.Encode(s.trSignKP)
	if err != nil {
		return nil, fmt.Errorf("natsbus.auth: encode user JWT: %w", err)
	}
	creds, err := jwt.FormatUserConfig(userJWT, userSeed)
	if err != nil {
		return nil, fmt.Errorf("natsbus.auth: format creds: %w", err)
	}
	if err := writeFile(s.CredsPath(source), creds, 0o600); err != nil {
		return nil, err
	}
	if err := s.store.SetSourceUserPubKey(ctx, source, userPub); err != nil {
		return nil, err
	}

	if oldPub != "" && oldPub != userPub {
		if err := s.revokeLocked(oldPub); err != nil {
			return nil, err
		}
	} else {
		if err := s.refreshAccountClaimsLocked(); err != nil {
			return nil, err
		}
	}
	return creds, nil
}

// RevokeSource revokes the current user for source. No-op if no
// pubkey was recorded for it.
func (s *AuthStore) RevokeSource(ctx context.Context, source string) error {
	if s.store == nil {
		return fmt.Errorf("natsbus.auth: RevokeSource requires a PubKeyStore")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pub, err := s.store.GetSourceUserPubKey(ctx, source)
	if err != nil {
		return err
	}
	if pub == "" {
		return nil
	}
	return s.revokeLocked(pub)
}

func (s *AuthStore) revokeLocked(userPub string) error {
	claims, err := jwt.DecodeAccountClaims(s.trinityJWT)
	if err != nil {
		return fmt.Errorf("natsbus.auth: decode TRINITY JWT: %w", err)
	}
	claims.Revoke(userPub)
	return s.encodeAndStoreTrinityLocked(claims)
}

func (s *AuthStore) refreshAccountClaimsLocked() error {
	claims, err := jwt.DecodeAccountClaims(s.trinityJWT)
	if err != nil {
		return fmt.Errorf("natsbus.auth: decode TRINITY JWT: %w", err)
	}
	return s.encodeAndStoreTrinityLocked(claims)
}

func (s *AuthStore) encodeAndStoreTrinityLocked(claims *jwt.AccountClaims) error {
	newJWT, err := claims.Encode(s.opSignKP)
	if err != nil {
		return fmt.Errorf("natsbus.auth: encode TRINITY JWT: %w", err)
	}
	s.trinityJWT = newJWT
	if err := writeFile(filepath.Join(s.dir, authSubdir, "trinity.jwt"), []byte(newJWT), 0o600); err != nil {
		return err
	}
	trPub, _ := s.trinityKP.PublicKey()
	if err := s.resolver.Store(trPub, newJWT); err != nil {
		return fmt.Errorf("natsbus.auth: resolver store TRINITY: %w", err)
	}
	if s.runningSrv != nil {
		if acc, _ := s.runningSrv.LookupAccount(trPub); acc != nil {
			s.runningSrv.UpdateAccountClaims(acc, claims)
		}
	}
	return nil
}

func (s *AuthStore) buildInternalCreds(path string) ([]byte, error) {
	userKP, err := nkeys.CreateUser()
	if err != nil {
		return nil, fmt.Errorf("natsbus.auth: create internal user kp: %w", err)
	}
	userPub, _ := userKP.PublicKey()
	userSeed, _ := userKP.Seed()

	uc := jwt.NewUserClaims(userPub)
	uc.Name = "trinity-hub-internal"
	trPub, _ := s.trinityKP.PublicKey()
	uc.IssuerAccount = trPub
	uc.Permissions.Pub.Allow.Add(">")
	uc.Permissions.Sub.Allow.Add(">")

	userJWT, err := uc.Encode(s.trSignKP)
	if err != nil {
		return nil, fmt.Errorf("natsbus.auth: encode internal user JWT: %w", err)
	}
	creds, err := jwt.FormatUserConfig(userJWT, userSeed)
	if err != nil {
		return nil, fmt.Errorf("natsbus.auth: format internal creds: %w", err)
	}
	if err := writeFile(path, creds, 0o600); err != nil {
		return nil, err
	}
	return creds, nil
}

// InboxPrefixFor is the per-source NATS inbox prefix. A collector sets
// its nats.CustomInboxPrefix to this string; the hub issues its NKey
// with Sub permission scoped to "<prefix>.>". Without this scoping any
// collector could subscribe to _INBOX.> and harvest other collectors'
// RPC replies (e.g. link codes).
func InboxPrefixFor(sourceID string) string {
	return "_INBOX." + sourceID
}

func addSourcePermissions(uc *jwt.UserClaims, sourceID string) {
	uc.Permissions.Pub.Allow.Add(
		"trinity.events."+sourceID+".>",
		"trinity.events."+sourceID,
		"trinity.register."+sourceID+".>",
		"trinity.register."+sourceID,
		"trinity.live."+sourceID+".>",
		"trinity.live."+sourceID,
		"trinity.rpc.greet."+sourceID+".>",
		"trinity.rpc.greet."+sourceID,
		"trinity.rpc.claim."+sourceID+".>",
		"trinity.rpc.claim."+sourceID,
		"trinity.rpc.link."+sourceID+".>",
		"trinity.rpc.link."+sourceID,
		"trinity.rpc.server.register."+sourceID+".>",
		"trinity.rpc.server.register."+sourceID,
		"trinity.rpc.identity.upsert."+sourceID+".>",
		"trinity.rpc.identity.upsert."+sourceID,
		"trinity.rpc.identity.upsert_bot."+sourceID+".>",
		"trinity.rpc.identity.upsert_bot."+sourceID,
		"trinity.rpc.identity.lookup."+sourceID+".>",
		"trinity.rpc.identity.lookup."+sourceID,
		"trinity.rpc.source.progress."+sourceID+".>",
		"trinity.rpc.source.progress."+sourceID,
		// _INBOX.> lets the collector m.Respond() to a hub-issued RCON
		// proxy request. NATS request-reply uses random reply tokens
		// under _INBOX.<random>, so the broad rule is fine here — the
		// collector still can't invent its own request to a known
		// inbox token (those are unguessable per-call).
		"_INBOX.>",
	)
	uc.Permissions.Sub.Allow.Add(InboxPrefixFor(sourceID) + ".>")
	// Hub → collector RCON proxy: collector listens on
	// trinity.rcon.exec.<sourceID>. Scoped to the collector's own
	// source so a malicious collector can't intercept requests
	// addressed to a different source.
	uc.Permissions.Sub.Allow.Add(RconExecSubjectPrefix + sourceID)
}

func loadOrCreateSeed(path string, ctor func() (nkeys.KeyPair, error)) (nkeys.KeyPair, error) {
	if b, err := os.ReadFile(path); err == nil {
		kp, err := nkeys.FromSeed(b)
		if err != nil {
			return nil, fmt.Errorf("natsbus.auth: parse seed %s: %w", path, err)
		}
		return kp, nil
	}
	kp, err := ctor()
	if err != nil {
		return nil, fmt.Errorf("natsbus.auth: create key (%s): %w", path, err)
	}
	seed, _ := kp.Seed()
	if err := writeFile(path, seed, 0o600); err != nil {
		return nil, err
	}
	return kp, nil
}

func loadOrCreateOperatorJWT(path string, opKP, opSignKP, sysKP nkeys.KeyPair) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		return string(b), nil
	}
	opPub, _ := opKP.PublicKey()
	signPub, _ := opSignKP.PublicKey()
	sysPub, _ := sysKP.PublicKey()
	claims := jwt.NewOperatorClaims(opPub)
	claims.Name = "trinity"
	claims.SigningKeys.Add(signPub)
	claims.SystemAccount = sysPub
	tok, err := claims.Encode(opKP)
	if err != nil {
		return "", fmt.Errorf("natsbus.auth: encode operator JWT: %w", err)
	}
	if err := writeFile(path, []byte(tok), 0o600); err != nil {
		return "", err
	}
	return tok, nil
}

func loadOrCreateAccountJWT(path string, accKP, opSignKP nkeys.KeyPair, name string, signingKeys []string) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		return string(b), nil
	}
	pub, _ := accKP.PublicKey()
	claims := jwt.NewAccountClaims(pub)
	claims.Name = name
	for _, sk := range signingKeys {
		claims.SigningKeys.Add(sk)
	}
	// -1 limits = unlimited in jwt v2.
	if name == "TRINITY" {
		claims.Limits.JetStreamLimits.DiskStorage = -1
		claims.Limits.JetStreamLimits.MemoryStorage = -1
		claims.Limits.JetStreamLimits.Streams = -1
		claims.Limits.JetStreamLimits.Consumer = -1
	}
	tok, err := claims.Encode(opSignKP)
	if err != nil {
		return "", fmt.Errorf("natsbus.auth: encode %s account JWT: %w", name, err)
	}
	if err := writeFile(path, []byte(tok), 0o600); err != nil {
		return "", err
	}
	return tok, nil
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("natsbus.auth: mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("natsbus.auth: write %s: %w", path, err)
	}
	return nil
}

