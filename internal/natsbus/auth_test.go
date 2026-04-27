package natsbus_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/natsbus"
)

// startAuthRig boots a hub server with JWT auth enabled and returns
// the server and a temp directory. The caller mints its own creds
// via s.Auth().MintUserCreds.
func startAuthRig(t *testing.T) (*natsbus.Server, string) {
	t.Helper()
	tmp := t.TempDir()
	port := freePort(t)
	cfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub: &config.HubConfig{
			DedupWindow: config.Duration(time.Minute),
			Retention:   config.Duration(time.Hour),
		},
	}
	s, err := natsbus.Start(cfg, tmp)
	if err != nil {
		t.Fatalf("natsbus.Start: %v", err)
	}
	t.Cleanup(s.Stop)
	return s, tmp
}

func TestAuthUserCanPublishOwnSubject(t *testing.T) {
	s, _ := startAuthRig(t)
	_, err := s.Auth().MintUserCreds("alpha")
	if err != nil {
		t.Fatalf("MintUserCreds: %v", err)
	}
	credsPath := s.Auth().CredsPath("alpha")
	nc, err := nats.Connect(s.ClientURL(), nats.UserCredentials(credsPath))
	if err != nil {
		t.Fatalf("connect alpha: %v", err)
	}
	defer nc.Close()
	if err := nc.Publish("trinity.live.alpha", []byte("ok")); err != nil {
		t.Fatalf("publish own subject: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
}

func TestAuthUserCannotPublishOtherSubject(t *testing.T) {
	s, _ := startAuthRig(t)
	if _, err := s.Auth().MintUserCreds("alpha"); err != nil {
		t.Fatalf("MintUserCreds alpha: %v", err)
	}
	credsPath := s.Auth().CredsPath("alpha")

	// Subscribe as hub-internal (full perms) to detect whether the
	// publish actually reached the wire. If JWT perms are enforced,
	// the subscriber should never see the message.
	adminNC, err := s.ConnectInternal()
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer adminNC.Close()
	received := make(chan struct{}, 1)
	sub, err := adminNC.Subscribe("trinity.live.beta", func(*nats.Msg) { received <- struct{}{} })
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()
	if err := adminNC.Flush(); err != nil {
		t.Fatalf("admin flush: %v", err)
	}

	// Alpha's user tries to publish onto Beta's subject. The server
	// should reject the publish at the perms check (or detect it as
	// a permissions violation asynchronously). Either way, the admin
	// subscriber should receive nothing.
	alphaNC, err := nats.Connect(s.ClientURL(), nats.UserCredentials(credsPath))
	if err != nil {
		t.Fatalf("alpha connect: %v", err)
	}
	defer alphaNC.Close()
	_ = alphaNC.Publish("trinity.live.beta", []byte("nope"))
	_ = alphaNC.Flush()

	select {
	case <-received:
		t.Fatal("beta received a message from alpha — permissions not enforced")
	case <-time.After(200 * time.Millisecond):
		// expected: rejected by server permissions
	}
}

// Without per-source inbox scoping a collector could subscribe to
// _INBOX.> and harvest other collectors' RPC replies, including
// 10-minute-valid link codes. This test confirms the Sub permission
// is scoped so source alpha cannot subscribe under source beta's
// inbox prefix.
func TestAuthUserCannotSubscribeAcrossInboxScopes(t *testing.T) {
	s, _ := startAuthRig(t)
	if _, err := s.Auth().MintUserCreds("alpha"); err != nil {
		t.Fatalf("mint alpha: %v", err)
	}
	if _, err := s.Auth().MintUserCreds("beta"); err != nil {
		t.Fatalf("mint beta: %v", err)
	}

	alphaNC, err := nats.Connect(s.ClientURL(),
		nats.UserCredentials(s.Auth().CredsPath("alpha")),
		nats.CustomInboxPrefix(natsbus.InboxPrefixFor("alpha")),
	)
	if err != nil {
		t.Fatalf("alpha connect: %v", err)
	}
	defer alphaNC.Close()

	permErr := make(chan error, 1)
	alphaNC.SetErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, e error) {
		select {
		case permErr <- e:
		default:
		}
	})

	if _, err := alphaNC.SubscribeSync(natsbus.InboxPrefixFor("beta") + ".>"); err != nil {
		return
	}
	if err := alphaNC.Flush(); err != nil {
		t.Fatalf("alpha flush: %v", err)
	}
	select {
	case e := <-permErr:
		if e == nil {
			t.Fatal("expected permissions violation, got nil")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected permissions violation on cross-source inbox subscription")
	}
}

func TestAuthRotationRevokesOldCreds(t *testing.T) {
	s, _ := startAuthRig(t)
	if _, err := s.Auth().MintUserCreds("gamma"); err != nil {
		t.Fatalf("mint v1: %v", err)
	}
	credsPath := s.Auth().CredsPath("gamma")
	oldCreds, err := os.ReadFile(credsPath)
	if err != nil {
		t.Fatalf("read old creds: %v", err)
	}

	// Open a connection under the old creds. It should be active now.
	oldNC, err := nats.Connect(s.ClientURL(), nats.UserCredentials(credsPath))
	if err != nil {
		t.Fatalf("old connect: %v", err)
	}
	defer oldNC.Close()

	// Rotate (mint again for the same source). The old user pubkey
	// must end up in the TRINITY account's revocation list.
	if _, err := s.Auth().MintUserCreds("gamma"); err != nil {
		t.Fatalf("mint v2 (rotation): %v", err)
	}
	newCreds, err := os.ReadFile(credsPath)
	if err != nil {
		t.Fatalf("read new creds: %v", err)
	}
	if string(oldCreds) == string(newCreds) {
		t.Fatal("rotation produced identical creds — new NKey was not generated")
	}

	// The old client's publish should now fail; the server revokes
	// active connections on UpdateAccountClaims. Use a fresh connect
	// attempt with the OLD creds file contents (restored) to avoid
	// racing with the async disconnect — writing the old bytes back
	// into a temp file and connecting from there.
	oldFile := t.TempDir() + "/old.creds"
	if err := os.WriteFile(oldFile, oldCreds, 0o600); err != nil {
		t.Fatalf("restore old creds: %v", err)
	}
	// Give the server a moment to process the revocation propagation.
	time.Sleep(50 * time.Millisecond)
	if nc, err := nats.Connect(s.ClientURL(), nats.UserCredentials(oldFile), nats.Timeout(500*time.Millisecond), nats.MaxReconnects(0)); err == nil {
		nc.Close()
		t.Error("reconnect with revoked creds succeeded; expected auth failure")
	}

	// The NEW creds should still work.
	newNC, err := nats.Connect(s.ClientURL(), nats.UserCredentials(credsPath))
	if err != nil {
		t.Fatalf("new creds connect: %v", err)
	}
	defer newNC.Close()
	if err := newNC.Publish("trinity.live.gamma", []byte("ok")); err != nil {
		t.Fatalf("new creds publish: %v", err)
	}
}
