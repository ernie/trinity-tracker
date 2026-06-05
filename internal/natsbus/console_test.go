package natsbus_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/console"
	"github.com/ernie/trinity-tracker/internal/natsbus"
	"github.com/nats-io/nats.go"
)

type allowAllConsole struct{}

func (allowAllConsole) AuthorizeConsole(string, natsbus.RconRole) error { return nil }

type denyConsole struct{}

func (denyConsole) AuthorizeConsole(string, natsbus.RconRole) error {
	return fmt.Errorf("delegation disabled")
}

func startConsoleBus(t *testing.T) (*nats.Conn, *nats.Conn) {
	t.Helper()
	cfg := &config.TrackerConfig{
		NATS: config.NATSConfig{URL: fmt.Sprintf("nats://127.0.0.1:%d", freePort(t))},
		Hub:  &config.HubConfig{},
	}
	ns, err := natsbus.Start(cfg, t.TempDir(), newMemPubKeyStore())
	if err != nil {
		t.Fatalf("natsbus.Start: %v", err)
	}
	t.Cleanup(func() { ns.Stop() })
	hubNC, err := ns.ConnectInternal(nats.Name("hub"))
	if err != nil {
		t.Fatalf("hub connect: %v", err)
	}
	t.Cleanup(hubNC.Close)
	colNC, err := ns.ConnectInternal(nats.Name("collector"))
	if err != nil {
		t.Fatalf("collector connect: %v", err)
	}
	t.Cleanup(colNC.Close)
	return hubNC, colNC
}

func TestConsoleWatchLeaseAndLines(t *testing.T) {
	hubNC, colNC := startConsoleBus(t)

	reg := console.NewRegistry()
	ring := reg.Ring("ffa")
	ring.SetTapUp(true)
	ring.Append("scroll-1")
	ring.Append("scroll-2")

	fwd, err := natsbus.RegisterConsoleForwarder(colNC, "src", reg, allowAllConsole{})
	if err != nil {
		t.Fatalf("RegisterConsoleForwarder: %v", err)
	}
	defer fwd.Stop()

	client, err := natsbus.NewConsoleClient(hubNC)
	if err != nil {
		t.Fatalf("NewConsoleClient: %v", err)
	}

	var mu sync.Mutex
	var got []console.Line
	sub, err := client.SubscribeLines("src", "ffa", func(b natsbus.ConsoleLineBatch) {
		mu.Lock()
		got = append(got, b.Lines...)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("SubscribeLines: %v", err)
	}
	defer sub.Unsubscribe()
	hubNC.Flush()

	reply, err := client.Watch(context.Background(), "src", natsbus.ConsoleWatchRequest{
		ServerKey: "ffa", Username: "ernie", Role: natsbus.RconRoleOwner, TTLMs: 500,
	})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if !reply.TapUp || len(reply.Scrollback) != 2 || reply.Scrollback[0].Text != "scroll-1" {
		t.Errorf("watch reply: %+v", reply)
	}

	ring.Append("live-1")
	ring.Append("live-2")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	if len(got) < 2 || got[0].Text != "live-1" || got[1].Text != "live-2" {
		t.Fatalf("live lines: %+v", got)
	}
	baseline := len(got)
	mu.Unlock()

	// Lease (500ms) expires: lines stop flowing.
	time.Sleep(800 * time.Millisecond)
	ring.Append("after-expiry")
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	if len(got) != baseline {
		t.Errorf("lines flowed after lease expiry: %+v", got[baseline:])
	}
	mu.Unlock()

	// Re-watch: a fresh pump resumes, with the line in scrollback.
	reply, err = client.Watch(context.Background(), "src", natsbus.ConsoleWatchRequest{
		ServerKey: "ffa", Username: "ernie", Role: natsbus.RconRoleOwner, TTLMs: 500,
	})
	if err != nil {
		t.Fatalf("re-watch: %v", err)
	}
	if last := reply.Scrollback[len(reply.Scrollback)-1]; last.Text != "after-expiry" {
		t.Errorf("re-watch scrollback tail: %+v", last)
	}
	ring.Append("live-3")
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > baseline {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	if len(got) == baseline || got[len(got)-1].Text != "live-3" {
		t.Errorf("no lines after re-watch: %+v", got)
	}
	mu.Unlock()
}

func TestConsoleWatchDenied(t *testing.T) {
	hubNC, colNC := startConsoleBus(t)

	reg := console.NewRegistry()
	reg.Ring("ffa")
	fwd, err := natsbus.RegisterConsoleForwarder(colNC, "src", reg, denyConsole{})
	if err != nil {
		t.Fatalf("RegisterConsoleForwarder: %v", err)
	}
	defer fwd.Stop()

	client, _ := natsbus.NewConsoleClient(hubNC)
	_, err = client.Watch(context.Background(), "src", natsbus.ConsoleWatchRequest{
		ServerKey: "ffa", Role: natsbus.RconRoleHubAdmin, TTLMs: 500,
	})
	if err == nil {
		t.Fatal("watch succeeded despite denial")
	}

	// Unknown key also errors.
	fwd2, err := natsbus.RegisterConsoleForwarder(colNC, "src2", reg, allowAllConsole{})
	if err != nil {
		t.Fatalf("RegisterConsoleForwarder: %v", err)
	}
	defer fwd2.Stop()
	_, err = client.Watch(context.Background(), "src2", natsbus.ConsoleWatchRequest{
		ServerKey: "nope", Role: natsbus.RconRoleOwner, TTLMs: 500,
	})
	if err == nil {
		t.Fatal("watch succeeded for unknown key")
	}
}
