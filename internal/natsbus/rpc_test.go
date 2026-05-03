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

func startRPCRig(t *testing.T) (*natsbus.RPCClient, *storage.Store) {
	t.Helper()
	port := rpcFreePort(t)
	trackerCfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", port)},
		Hub:  &config.HubConfig{DedupWindow: config.Duration(time.Minute), Retention: config.Duration(time.Hour)},
	}
	ns, err := natsbus.Start(trackerCfg, t.TempDir(), nil)
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

	serverNC, err := ns.ConnectInternal(nats.Name("rpc-server"))
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	rpcSrv, err := natsbus.RegisterRPCHandlers(serverNC, writer)
	if err != nil {
		t.Fatalf("RegisterRPCHandlers: %v", err)
	}

	clientNC, err := ns.ConnectInternal(nats.Name("rpc-client"))
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
	reply, err := client.Claim(context.Background(), hub.ClaimRequest{GUID: "anyguid"})
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
}

func TestRPCClaimRoundTripGeneratesCode(t *testing.T) {
	client, store := startRPCRig(t)
	ctx := context.Background()

	if _, err := store.UpsertPlayerGUID(ctx, "guid-a", "Ernie", "ernie", time.Now().UTC(), false); err != nil {
		t.Fatalf("seed player: %v", err)
	}
	reply, err := client.Claim(ctx, hub.ClaimRequest{GUID: "guid-a"})
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

func TestRPCRegisterServerUpsertsAndTags(t *testing.T) {
	client, store := startRPCRig(t)
	ctx := context.Background()

	srv, err := client.RegisterServer(ctx, "", "ffa", "example.com:27960")
	if err != nil {
		t.Fatalf("RegisterServer: %v", err)
	}
	if srv == nil || srv.ID == 0 {
		t.Fatalf("got nil/zero server: %+v", srv)
	}
	if srv.Key != "ffa" {
		t.Errorf("Name = %q, want %q", srv.Key, "ffa")
	}
	// The handler calls TagLocalServerSource with (source, local_id=servers.id)
	// so envelope resolution round-trips.
	got, err := store.ResolveServerIDForSource(ctx, "test-source", srv.ID)
	if err != nil {
		t.Fatalf("ResolveServerIDForSource: %v", err)
	}
	if got != srv.ID {
		t.Errorf("resolved = %d, want %d", got, srv.ID)
	}
}

func TestRPCIdentityRoundTrip(t *testing.T) {
	client, _ := startRPCRig(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	id, err := client.UpsertPlayerIdentity(ctx, "guid-human", "Ernie", "ernie", ts, false)
	if err != nil {
		t.Fatalf("UpsertPlayerIdentity: %v", err)
	}
	if !id.Found || id.PlayerID == 0 {
		t.Fatalf("UpsertPlayerIdentity returned %+v", id)
	}

	bot, err := client.UpsertBotPlayerIdentity(ctx, "Sarge", "sarge", ts)
	if err != nil {
		t.Fatalf("UpsertBotPlayerIdentity: %v", err)
	}
	if !bot.Found || bot.PlayerID == 0 {
		t.Fatalf("bot identity: %+v", bot)
	}

	found, err := client.LookupPlayerIdentity(ctx, "guid-human")
	if err != nil {
		t.Fatalf("LookupPlayerIdentity: %v", err)
	}
	if found.PlayerID != id.PlayerID {
		t.Errorf("lookup PlayerID = %d, want %d", found.PlayerID, id.PlayerID)
	}

	missing, err := client.LookupPlayerIdentity(ctx, "guid-missing")
	if err != nil {
		t.Fatalf("LookupPlayerIdentity(missing): %v", err)
	}
	if missing.Found {
		t.Errorf("expected Found=false for unknown GUID, got %+v", missing)
	}
}

