package natsbus_test

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/hub"
	"github.com/ernie/trinity-tracker/internal/natsbus"
	"github.com/ernie/trinity-tracker/internal/storage"
)

func rpcFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func startRPCRig(t *testing.T) (hub.RPCClient, *storage.Store) {
	t.Helper()
	port := rpcFreePort(t)
	trackerCfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub:  &config.HubConfig{DedupWindow: config.Duration(time.Minute), Retention: config.Duration(time.Hour)},
	}
	ns, err := natsbus.Start(trackerCfg, t.TempDir())
	if err != nil {
		t.Fatalf("natsbus.Start: %v", err)
	}

	store, err := storage.New(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	writer := hub.NewWriter(store)
	writer.Start(ctx)

	serverNC, err := nats.Connect("", nats.InProcessServer(ns.NATSServer()), nats.Name("rpc-server"))
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	rpcSrv, err := natsbus.RegisterRPCHandlers(serverNC, writer)
	if err != nil {
		t.Fatalf("RegisterRPCHandlers: %v", err)
	}

	clientNC, err := nats.Connect("", nats.InProcessServer(ns.NATSServer()), nats.Name("rpc-client"))
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	client, err := natsbus.NewRPCClient(clientNC, "test-source", 0)
	if err != nil {
		t.Fatalf("NewRPCClient: %v", err)
	}

	// Teardown in dependency order (LIFO registration).
	t.Cleanup(func() { ns.Stop() })
	t.Cleanup(func() { _ = store.Close() })
	t.Cleanup(func() { clientNC.Close() })
	t.Cleanup(func() { serverNC.Close() })
	t.Cleanup(func() { writer.Stop() })
	t.Cleanup(cancel)
	t.Cleanup(rpcSrv.Stop)

	return client, store
}

func TestRPCClaimUnknownPlayer(t *testing.T) {
	client, _ := startRPCRig(t)
	reply, err := client.Claim(context.Background(), hub.ClaimRequest{GUID: "anyguid", PlayerID: 0})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if reply.Status != hub.ClaimUnknownPlayer {
		t.Errorf("Status = %q, want %q", reply.Status, hub.ClaimUnknownPlayer)
	}
}

func TestRPCLinkInvalidFormat(t *testing.T) {
	client, _ := startRPCRig(t)
	reply, err := client.Link(context.Background(), hub.LinkRequest{GUID: "anyguid", Code: "nope"})
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if reply.Status != hub.LinkInvalidFormat {
		t.Errorf("Status = %q, want %q", reply.Status, hub.LinkInvalidFormat)
	}
}

func TestRPCLinkInvalidCode(t *testing.T) {
	client, _ := startRPCRig(t)
	reply, err := client.Link(context.Background(), hub.LinkRequest{GUID: "anyguid", Code: "123456"})
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if reply.Status != hub.LinkInvalidCode {
		t.Errorf("Status = %q, want %q", reply.Status, hub.LinkInvalidCode)
	}
}

func TestRPCGreetUnknownGUIDReturnsUnauthenticated(t *testing.T) {
	client, _ := startRPCRig(t)
	reply, err := client.Greet(context.Background(), hub.GreetRequest{GUID: "unseen-guid"})
	if err != nil {
		t.Fatalf("Greet: %v", err)
	}
	if reply.AuthResult != hub.AuthUnauthenticated {
		t.Errorf("AuthResult = %q, want %q", reply.AuthResult, hub.AuthUnauthenticated)
	}
	if reply.PlayerID != 0 {
		t.Errorf("PlayerID = %d, want 0", reply.PlayerID)
	}
}

func TestRPCClaimRoundTripGeneratesCode(t *testing.T) {
	client, store := startRPCRig(t)
	ctx := context.Background()

	pg, err := store.UpsertPlayerGUID(ctx, "guid-a", "Ernie", "ernie", time.Now().UTC(), false)
	if err != nil {
		t.Fatalf("seed player: %v", err)
	}
	reply, err := client.Claim(ctx, hub.ClaimRequest{GUID: "guid-a", PlayerID: pg.PlayerID})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if reply.Status != hub.ClaimOK {
		t.Fatalf("Status = %q, want %q (message=%q)", reply.Status, hub.ClaimOK, reply.Message)
	}
	if reply.Code == "" {
		t.Error("Code empty")
	}
	if reply.ExpiresAt.Before(time.Now()) {
		t.Errorf("ExpiresAt in past: %v", reply.ExpiresAt)
	}
}
