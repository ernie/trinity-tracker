package directory

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/storage"
)

// startTestServer spins up a directory.Server on a random localhost port
// with the supplied gate store. Returns the server and its UDP addr.
func startTestServer(t *testing.T, store gateStore) (*Server, netip.AddrPort, context.CancelFunc) {
	t.Helper()
	port := freeUDPPort(t)
	srv, err := New(Config{
		ListenAddr:       "127.0.0.1",
		Port:             port,
		HeartbeatExpiry:  15 * time.Minute,
		ChallengeTimeout: 2 * time.Second,
		GateRefresh:      100 * time.Millisecond,
		MaxServers:       16,
		Store:            store,
		Conns:            fakeGateConns{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	addr := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port))
	select {
	case <-srv.started:
	case <-time.After(2 * time.Second):
		t.Fatal("server never reached started")
	}
	t.Cleanup(func() {
		cancel()
		srv.Stop()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Log("directory server did not stop in time")
		}
	})
	return srv, addr, cancel
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	port := c.LocalAddr().(*net.UDPAddr).Port
	c.Close()
	return port
}

func TestServerHeartbeatGateAcceptsAndChallenges(t *testing.T) {
	// Spin up a fake "Q3 server" socket, then point the gate at it.
	q3conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("q3 socket: %v", err)
	}
	defer q3conn.Close()
	q3Addr := q3conn.LocalAddr().(*net.UDPAddr)

	store := &fakeGateStore{
		rows: []storage.RemoteServer{
			{ID: 99, Source: "local", Key: "x", Address: q3Addr.String()},
		},
	}

	srv, dirAddr, _ := startTestServer(t, store)

	// Wait for the gate to load.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && srv.gate.Size() == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if srv.gate.Size() == 0 {
		t.Fatal("gate never populated")
	}

	// Send a heartbeat.
	if _, err := q3conn.WriteToUDP([]byte(OOBHeader+"heartbeat QuakeArena-1\n"),
		net.UDPAddrFromAddrPort(dirAddr)); err != nil {
		t.Fatalf("send heartbeat: %v", err)
	}

	// Expect a getinfo probe back.
	q3conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1500)
	n, _, err := q3conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read probe: %v", err)
	}
	got := string(buf[:n])
	if !strings.HasPrefix(got, OOBHeader+"getinfo ") {
		t.Fatalf("expected getinfo probe, got %q", got)
	}
	if srv.metrics.probesSent.Load() != 1 {
		t.Errorf("probesSent=%d, want 1", srv.metrics.probesSent.Load())
	}

	// Extract the challenge and reply with infoResponse.
	challenge := strings.TrimSpace(strings.TrimPrefix(got, OOBHeader+"getinfo "))
	info := fmt.Sprintf("%sinfoResponse\n\\challenge\\%s\\protocol\\68\\engine\\trinity-engine/0.4.2\\hostname\\Test\\sv_maxclients\\16\\clients\\2\\gamename\\baseq3",
		OOBHeader, challenge)
	if _, err := q3conn.WriteToUDP([]byte(info), net.UDPAddrFromAddrPort(dirAddr)); err != nil {
		t.Fatalf("send infoResponse: %v", err)
	}

	// Wait for the registry to land.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && srv.registry.Len() == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if srv.registry.Len() != 1 {
		t.Fatalf("registry size = %d, want 1", srv.registry.Len())
	}

	// Now ask for a list. Use a third socket.
	clientConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("client socket: %v", err)
	}
	defer clientConn.Close()
	if _, err := clientConn.WriteToUDP([]byte(OOBHeader+"getservers 68\n"),
		net.UDPAddrFromAddrPort(dirAddr)); err != nil {
		t.Fatalf("send getservers: %v", err)
	}
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err = clientConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read getservers response: %v", err)
	}
	resp := buf[:n]
	if !bytes.HasPrefix(resp, []byte(OOBHeader+"getserversResponse")) {
		t.Fatalf("bad response prefix: %q", resp)
	}
	if !bytes.HasSuffix(resp, []byte("\\EOT\x00\x00\x00")) {
		t.Error("missing EOT")
	}
	// We expect at least one record (ours).
	body := resp[len(OOBHeader+"getserversResponse"):]
	if len(body) <= len("\\EOT\x00\x00\x00") {
		t.Fatal("no records returned")
	}
}

func TestServerHeartbeatRejectsUnregisteredAddr(t *testing.T) {
	store := &fakeGateStore{rows: nil}
	srv, dirAddr, _ := startTestServer(t, store)

	// Wait for the gate to load (empty).
	time.Sleep(200 * time.Millisecond)

	q3conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("q3 socket: %v", err)
	}
	defer q3conn.Close()
	if _, err := q3conn.WriteToUDP([]byte(OOBHeader+"heartbeat QuakeArena-1\n"),
		net.UDPAddrFromAddrPort(dirAddr)); err != nil {
		t.Fatalf("send: %v", err)
	}
	q3conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1500)
	if _, _, err := q3conn.ReadFromUDP(buf); err == nil {
		t.Error("expected no probe (gate should reject), got a packet")
	}
	if srv.metrics.heartbeatsRejected.Load() == 0 {
		t.Error("heartbeatsRejected counter never incremented")
	}
}

func TestServerInfoResponseRejectsNonTrinityEngine(t *testing.T) {
	q3conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("q3 socket: %v", err)
	}
	defer q3conn.Close()
	q3Addr := q3conn.LocalAddr().(*net.UDPAddr)

	store := &fakeGateStore{
		rows: []storage.RemoteServer{
			{ID: 7, Address: q3Addr.String()},
		},
	}
	srv, dirAddr, _ := startTestServer(t, store)
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && srv.gate.Size() == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	// Heartbeat — gate accepts, directory probes us.
	if _, err := q3conn.WriteToUDP([]byte(OOBHeader+"heartbeat QuakeArena-1\n"),
		net.UDPAddrFromAddrPort(dirAddr)); err != nil {
		t.Fatalf("send heartbeat: %v", err)
	}
	q3conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1500)
	n, _, err := q3conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read probe: %v", err)
	}
	got := string(buf[:n])
	challenge := strings.TrimSpace(strings.TrimPrefix(got, OOBHeader+"getinfo "))

	// Reply with an infoResponse that has no `engine` field — i.e.,
	// a stock ioquake3 server.
	info := fmt.Sprintf("%sinfoResponse\n\\challenge\\%s\\protocol\\68\\hostname\\Stock\\sv_maxclients\\16\\clients\\1\\gamename\\baseq3",
		OOBHeader, challenge)
	if _, err := q3conn.WriteToUDP([]byte(info), net.UDPAddrFromAddrPort(dirAddr)); err != nil {
		t.Fatalf("send infoResponse: %v", err)
	}

	// Give the dispatcher a moment, then assert no registration landed.
	time.Sleep(150 * time.Millisecond)
	if srv.registry.Len() != 0 {
		t.Errorf("registry size = %d, want 0 (non-trinity engine should be rejected)", srv.registry.Len())
	}
	if srv.metrics.heartbeatsRejected.Load() == 0 {
		t.Error("heartbeatsRejected counter should have ticked for engine mismatch")
	}
}

func TestServerInfoResponseAcceptsTrinityEngine(t *testing.T) {
	// Wire-level positive case for the engine check, in isolation
	// from the broader getservers flow tested elsewhere.
	q3conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("q3 socket: %v", err)
	}
	defer q3conn.Close()
	q3Addr := q3conn.LocalAddr().(*net.UDPAddr)

	store := &fakeGateStore{rows: []storage.RemoteServer{{ID: 8, Address: q3Addr.String()}}}
	srv, dirAddr, _ := startTestServer(t, store)
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && srv.gate.Size() == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	if _, err := q3conn.WriteToUDP([]byte(OOBHeader+"heartbeat QuakeArena-1\n"),
		net.UDPAddrFromAddrPort(dirAddr)); err != nil {
		t.Fatalf("send: %v", err)
	}
	q3conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1500)
	n, _, err := q3conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read probe: %v", err)
	}
	challenge := strings.TrimSpace(strings.TrimPrefix(string(buf[:n]), OOBHeader+"getinfo "))
	info := fmt.Sprintf("%sinfoResponse\n\\challenge\\%s\\protocol\\68\\engine\\trinity-engine/0.4.2\\sv_maxclients\\16\\clients\\1\\gamename\\baseq3",
		OOBHeader, challenge)
	if _, err := q3conn.WriteToUDP([]byte(info), net.UDPAddrFromAddrPort(dirAddr)); err != nil {
		t.Fatalf("send: %v", err)
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && srv.registry.Len() == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if srv.registry.Len() != 1 {
		t.Fatalf("registry size = %d, want 1", srv.registry.Len())
	}
	snap := srv.registry.Snapshot()
	if snap[0].engine != "trinity-engine/0.4.2" {
		t.Errorf("engine signature stored as %q, want trinity-engine/0.4.2", snap[0].engine)
	}
}

// fakeRegistryStore is an in-memory stand-in for storage.Store's
// directory_registrations methods. Captures Replace/Clear calls so
// tests can assert on what the server persisted at shutdown.
type fakeRegistryStore struct {
	rows     []storage.DirectoryRegistration
	cleared  int
	replaced int
}

func (f *fakeRegistryStore) ListDirectoryRegistrations(ctx context.Context) ([]storage.DirectoryRegistration, error) {
	out := make([]storage.DirectoryRegistration, len(f.rows))
	copy(out, f.rows)
	return out, nil
}

func (f *fakeRegistryStore) ReplaceDirectoryRegistrations(ctx context.Context, rows []storage.DirectoryRegistration) error {
	f.rows = make([]storage.DirectoryRegistration, len(rows))
	copy(f.rows, rows)
	f.replaced++
	return nil
}

func (f *fakeRegistryStore) ClearDirectoryRegistrations(ctx context.Context) error {
	f.rows = nil
	f.cleared++
	return nil
}

// startTestServerWithRegistry is startTestServer plus a registry
// store and a freshness window — the persistence-aware variant.
func startTestServerWithRegistry(t *testing.T, store gateStore, regStore registryStore, freshness time.Duration) (*Server, netip.AddrPort) {
	t.Helper()
	port := freeUDPPort(t)
	srv, err := New(Config{
		ListenAddr:         "127.0.0.1",
		Port:               port,
		HeartbeatExpiry:    15 * time.Minute,
		ChallengeTimeout:   2 * time.Second,
		GateRefresh:        100 * time.Millisecond,
		MaxServers:         16,
		Store:              store,
		Conns:              fakeGateConns{},
		RegistryStore:      regStore,
		PersistedFreshness: freshness,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	addr := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port))
	select {
	case <-srv.started:
	case <-time.After(2 * time.Second):
		t.Fatal("server never reached started")
	}
	t.Cleanup(func() {
		cancel()
		srv.Stop()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Log("directory server did not stop in time")
		}
	})
	return srv, addr
}

func TestServerRestoresFreshSnapshot(t *testing.T) {
	// Preload a fresh persisted entry whose addr is in the gate.
	q3Addr := netip.MustParseAddrPort("127.0.0.1:27960")
	gateStore := &fakeGateStore{rows: []storage.RemoteServer{
		{ID: 99, Source: "local", Key: "x", Address: q3Addr.String()},
	}}
	now := time.Now()
	regStore := &fakeRegistryStore{rows: []storage.DirectoryRegistration{{
		Addr: q3Addr.String(), ServerID: 99, Protocol: 68,
		Gamename: "baseq3", Engine: "trinity-engine/0.4.2",
		Clients: 2, MaxClients: 16, Gametype: 4,
		ValidatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(14 * time.Minute),
	}}}

	srv, _ := startTestServerWithRegistry(t, gateStore, regStore, 5*time.Minute)
	if srv.registry.Len() != 1 {
		t.Fatalf("registry size after restore = %d, want 1", srv.registry.Len())
	}
}

func TestServerCrashRecoveryIgnoresStaleSnapshot(t *testing.T) {
	// Persisted snapshot is older than the freshness window — the
	// "previous graceful shutdown was hours ago, must be a crash"
	// scenario. Registry should come up empty and the table cleared.
	q3Addr := netip.MustParseAddrPort("127.0.0.1:27960")
	gateStore := &fakeGateStore{rows: []storage.RemoteServer{
		{ID: 99, Source: "local", Key: "x", Address: q3Addr.String()},
	}}
	now := time.Now()
	regStore := &fakeRegistryStore{rows: []storage.DirectoryRegistration{{
		Addr: q3Addr.String(), ServerID: 99, Protocol: 68,
		Gamename: "baseq3", Engine: "trinity-engine/0.4.2",
		Clients: 2, MaxClients: 16, Gametype: 4,
		ValidatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-45 * time.Minute),
	}}}

	srv, _ := startTestServerWithRegistry(t, gateStore, regStore, 5*time.Minute)
	if srv.registry.Len() != 0 {
		t.Errorf("registry size after stale-restore = %d, want 0", srv.registry.Len())
	}
	if regStore.cleared == 0 {
		t.Error("ClearDirectoryRegistrations was not called for stale snapshot")
	}
	if len(regStore.rows) != 0 {
		t.Errorf("rows still populated after clear: %+v", regStore.rows)
	}
}

func TestServerPersistsOnStop(t *testing.T) {
	// Wire the full flow: heartbeat → challenge → infoResponse → registry.
	// Then stop the server and assert the entry was persisted.
	q3conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("q3 socket: %v", err)
	}
	defer q3conn.Close()
	q3Addr := q3conn.LocalAddr().(*net.UDPAddr)

	gateStore := &fakeGateStore{rows: []storage.RemoteServer{
		{ID: 99, Source: "local", Key: "x", Address: q3Addr.String()},
	}}
	regStore := &fakeRegistryStore{}

	port := freeUDPPort(t)
	srv, err := New(Config{
		ListenAddr:         "127.0.0.1",
		Port:               port,
		HeartbeatExpiry:    15 * time.Minute,
		ChallengeTimeout:   2 * time.Second,
		GateRefresh:        100 * time.Millisecond,
		MaxServers:         16,
		Store:              gateStore,
		Conns:              fakeGateConns{},
		RegistryStore:      regStore,
		PersistedFreshness: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	defer cancel()

	dirAddr := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port))

	// Wait for gate and listener.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && srv.gate.Size() == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	// Heartbeat → challenge → infoResponse.
	if _, err := q3conn.WriteToUDP([]byte(OOBHeader+"heartbeat QuakeArena-1\n"),
		net.UDPAddrFromAddrPort(dirAddr)); err != nil {
		t.Fatalf("send heartbeat: %v", err)
	}
	q3conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1500)
	n, _, err := q3conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read probe: %v", err)
	}
	challenge := strings.TrimSpace(strings.TrimPrefix(string(buf[:n]), OOBHeader+"getinfo "))
	info := fmt.Sprintf("%sinfoResponse\n\\challenge\\%s\\protocol\\68\\engine\\trinity-engine/0.4.2\\sv_maxclients\\16\\clients\\3\\gamename\\baseq3",
		OOBHeader, challenge)
	if _, err := q3conn.WriteToUDP([]byte(info), net.UDPAddrFromAddrPort(dirAddr)); err != nil {
		t.Fatalf("send infoResponse: %v", err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && srv.registry.Len() == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if srv.registry.Len() != 1 {
		t.Fatalf("registry size = %d, want 1", srv.registry.Len())
	}

	srv.Stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}

	if regStore.replaced == 0 {
		t.Fatal("ReplaceDirectoryRegistrations was never called on shutdown")
	}
	if len(regStore.rows) != 1 {
		t.Fatalf("persisted rows = %d, want 1", len(regStore.rows))
	}
	got := regStore.rows[0]
	if got.Addr != q3Addr.String() {
		t.Errorf("persisted addr = %q, want %q", got.Addr, q3Addr.String())
	}
	if got.Engine != "trinity-engine/0.4.2" || got.Clients != 3 || got.MaxClients != 16 || got.Protocol != 68 {
		t.Errorf("persisted row = %+v", got)
	}
}

func TestServerHeartbeatRejectsUnknownTag(t *testing.T) {
	q3conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("q3 socket: %v", err)
	}
	defer q3conn.Close()
	q3Addr := q3conn.LocalAddr().(*net.UDPAddr)

	store := &fakeGateStore{
		rows: []storage.RemoteServer{
			{ID: 99, Address: q3Addr.String()},
		},
	}
	srv, dirAddr, _ := startTestServer(t, store)
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && srv.gate.Size() == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	if _, err := q3conn.WriteToUDP([]byte(OOBHeader+"heartbeat DarkPlaces\n"),
		net.UDPAddrFromAddrPort(dirAddr)); err != nil {
		t.Fatalf("send: %v", err)
	}
	q3conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1500)
	if _, _, err := q3conn.ReadFromUDP(buf); err == nil {
		t.Error("expected no probe for unknown tag, got a packet")
	}
}
