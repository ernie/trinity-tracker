package natsbus

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nkeys"
)

// Directory layout under AuthDir(dataDir):
//
//	auth/
//	  operator.seed       identity key for the operator
//	  operator_sign.seed  operator signing key (signs account JWTs)
//	  operator.jwt        decoded at startup, fed into server.Options
//	  sys.seed            SYS account identity
//	  sys.jwt             SYS account JWT
//	  trinity.seed        TRINITY account identity
//	  trinity_sign.seed   TRINITY signing key (signs user JWTs)
//	  trinity.jwt         TRINITY account JWT (rewritten on revoke)
//	  hub_internal.creds  in-process hub client creds (admin-ish)
//	creds/
//	  <source_uuid>.creds collector user creds (delivered to operator)
//	  <source_uuid>.pub   cached user pubkey so rotations can revoke it

const (
	authSubdir  = "auth"
	credsSubdir = "creds"
	userPrefix  = "trinity"
)

// AuthStore owns the NKey/JWT material for the embedded NATS server's
// JWT auth and knows how to mint + revoke per-source user credentials
// on the running server.
type AuthStore struct {
	dir string

	mu           sync.Mutex
	opKP         nkeys.KeyPair
	opSignKP     nkeys.KeyPair
	sysKP        nkeys.KeyPair
	trinityKP    nkeys.KeyPair
	trSignKP     nkeys.KeyPair
	opJWT        string
	sysJWT       string
	trinityJWT   string
	opClaims     *jwt.OperatorClaims
	resolver     *server.MemAccResolver
	runningSrv   *server.Server
	internalCreds []byte
}

// LoadOrCreateAuthStore initializes the auth material under
// `<hubDataDir>/auth/` and prepares an in-memory account resolver
// loaded with the SYS + TRINITY account JWTs. On first run every seed
// and JWT is generated; subsequent runs load what's on disk.
func LoadOrCreateAuthStore(hubDataDir string) (*AuthStore, error) {
	s := &AuthStore{dir: hubDataDir}
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

	// Hub-internal client creds: the hub's own subscribers/RPC handlers
	// connect via nats.UserCredentials(InternalCredsPath). Granted full
	// pub/sub under TRINITY so every subject we already use keeps
	// working. The creds file is rewritten on every boot so a lost
	// operator.seed regenerates cleanly.
	if s.internalCreds, err = s.buildInternalCreds(filepath.Join(authDir, "hub_internal.creds")); err != nil {
		return nil, err
	}
	return s, nil
}

// TrustedOperators returns the operator claims to feed into
// server.Options.TrustedOperators.
func (s *AuthStore) TrustedOperators() []*jwt.OperatorClaims {
	return []*jwt.OperatorClaims{s.opClaims}
}

// SystemAccountPublicKey returns the SYS account pubkey for
// server.Options.SystemAccount.
func (s *AuthStore) SystemAccountPublicKey() string {
	pk, _ := s.sysKP.PublicKey()
	return pk
}

// Resolver returns the in-memory account resolver to set on
// server.Options.AccountResolver.
func (s *AuthStore) Resolver() *server.MemAccResolver {
	return s.resolver
}

// AttachServer records a running server so later Mint/Revoke calls can
// push live account-claim updates. Call once after natsbus.Start.
func (s *AuthStore) AttachServer(ns *server.Server) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runningSrv = ns
}

// InternalCredsPath returns the path to the hub-internal .creds file
// the hub's own NATS clients should use.
func (s *AuthStore) InternalCredsPath() string {
	return filepath.Join(s.dir, authSubdir, "hub_internal.creds")
}

// CredsPath returns the on-disk location of a source's user creds.
func (s *AuthStore) CredsPath(sourceUUID string) string {
	return filepath.Join(s.dir, credsSubdir, sourceUUID+".creds")
}

// MintUserCreds issues (or re-issues, revoking the old) a per-source
// user JWT with subject-scoped publish permissions, persists a .creds
// file under <dir>/creds/<source_uuid>.creds, and pushes the updated
// TRINITY account JWT + revocations to the running server if one is
// attached. Returns the creds file contents for immediate hand-off.
func (s *AuthStore) MintUserCreds(sourceID, sourceUUID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pubPath := filepath.Join(s.dir, credsSubdir, sourceUUID+".pub")
	var oldPub string
	if b, err := os.ReadFile(pubPath); err == nil {
		oldPub = string(b)
	}

	userKP, err := nkeys.CreateUser()
	if err != nil {
		return nil, fmt.Errorf("natsbus.auth: create user kp: %w", err)
	}
	userPub, _ := userKP.PublicKey()
	userSeed, _ := userKP.Seed()

	uc := jwt.NewUserClaims(userPub)
	uc.Name = userPrefix + "-" + sourceID
	trPub, _ := s.trinityKP.PublicKey()
	uc.IssuerAccount = trPub
	addSourcePermissions(uc, sourceID)

	userJWT, err := uc.Encode(s.trSignKP)
	if err != nil {
		return nil, fmt.Errorf("natsbus.auth: encode user JWT: %w", err)
	}
	creds, err := jwt.FormatUserConfig(userJWT, userSeed)
	if err != nil {
		return nil, fmt.Errorf("natsbus.auth: format creds: %w", err)
	}
	if err := writeFile(s.CredsPath(sourceUUID), creds, 0o600); err != nil {
		return nil, err
	}
	if err := writeFile(pubPath, []byte(userPub), 0o600); err != nil {
		return nil, err
	}

	if oldPub != "" && oldPub != userPub {
		if err := s.revokeLocked(oldPub); err != nil {
			return nil, err
		}
	} else {
		// Always refresh the TRINITY account JWT in the resolver so
		// newly-added signing-key rotations and revocation-list changes
		// reach the server.
		if err := s.refreshAccountClaimsLocked(); err != nil {
			return nil, err
		}
	}
	return creds, nil
}

// RevokeSource revokes whatever user is currently active for
// sourceUUID without issuing a replacement. No-op if nothing is
// recorded.
func (s *AuthStore) RevokeSource(sourceUUID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pubPath := filepath.Join(s.dir, credsSubdir, sourceUUID+".pub")
	b, err := os.ReadFile(pubPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return s.revokeLocked(string(b))
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

// addSourcePermissions grants publish on every subject the collector
// legitimately needs to send on, plus subscribe on inboxes (for RPC
// replies) and the collector's own live broadcast subjects (none
// today, but harmless).
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
	)
	// Inbox pattern for request/reply responses.
	uc.Permissions.Sub.Allow.Add("_INBOX.>")
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
	// Enable JetStream on TRINITY so the hub can declare its streams.
	// -1 on every numeric limit means "unlimited" in jwt v2; this
	// matches the in-process embedded server's no-limits baseline.
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

// ensure time import is used (for future TTLs on revocations if we
// decide to narrow the revocation window).
var _ = time.Now
