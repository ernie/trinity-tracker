package notify

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/discord"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
)

// fakePoller returns a fixed status for each server. Good enough for
// the activity-trigger tests — embed-body details are tested
// separately via the renderer's own assertions.
type fakePoller struct {
	byID map[int64]*domain.ServerStatus
}

func (f *fakePoller) GetServerStatus(id int64) *domain.ServerStatus {
	if f == nil {
		return nil
	}
	return f.byID[id]
}

// fakeTimer is the controllable Stopper used in tests. The notifier
// stores it on the per-server state and may call Stop() to cancel;
// the test calls fireAll() to simulate the timer elapsing.
type fakeTimer struct {
	fn      func()
	stopped bool
}

func (t *fakeTimer) Stop() bool {
	prev := t.stopped
	t.stopped = true
	return !prev
}

// fakeClock collects fakeTimer instances so a test can iterate
// pending fires in order.
type fakeClock struct {
	mu      sync.Mutex
	pending []*fakeTimer
}

func (c *fakeClock) after(_ time.Duration, fn func()) Stopper {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{fn: fn}
	c.pending = append(c.pending, t)
	return t
}

// fireAll runs every non-stopped pending fn. Older stopped timers
// stay in the list but are skipped — this mirrors how the notifier
// nils its tracked timer when it cancels, while the system timer
// might still exist transiently.
func (c *fakeClock) fireAll() {
	c.mu.Lock()
	pending := c.pending
	c.pending = nil
	c.mu.Unlock()
	for _, t := range pending {
		if !t.stopped {
			t.fn()
		}
	}
}

func (c *fakeClock) pendingLen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.pending {
		if !t.stopped {
			n++
		}
	}
	return n
}

// capturePoster records every embed POSTed so the test can assert on
// fire counts and order.
type capturePoster struct {
	mu     sync.Mutex
	embeds []discord.Embed
}

func (c *capturePoster) post(_ context.Context, _ string, embeds ...discord.Embed) error {
	c.mu.Lock()
	c.embeds = append(c.embeds, embeds...)
	c.mu.Unlock()
	return nil
}

func (c *capturePoster) titles() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.embeds))
	for i, e := range c.embeds {
		out[i] = e.Title
	}
	return out
}

func (c *capturePoster) waitFor(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		got := len(c.embeds)
		c.mu.Unlock()
		if got >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	c.mu.Lock()
	got := len(c.embeds)
	c.mu.Unlock()
	t.Fatalf("timed out waiting for %d embed(s), have %d", n, got)
}

// setup spins a fresh notifier wired to a hub.Presence we can
// manipulate directly to simulate the writer's RecordJoin/Leave
// before each OnHumanJoin/Leave invocation.
func setup(t *testing.T, serverID int64) (*Notifier, *hub.Presence, *fakeClock, *capturePoster, *fakePoller) {
	t.Helper()
	presence := hub.NewPresence()
	poster := &capturePoster{}
	clock := &fakeClock{}
	poller := &fakePoller{
		byID: map[int64]*domain.ServerStatus{
			serverID: {
				ServerID: serverID,
				Source:   "test",
				Key:      "alpha",
				Map:      "q3dm17",
				GameType: "FFA",
			},
		},
	}
	n := NewNotifier(
		ActivityConfig{
			WebhookURL:       "https://discord.com/api/webhooks/1/x",
			PublicURL:        "https://trinity.example.com",
			ActiveDelay:      30 * time.Second,
			InactiveDebounce: 60 * time.Second,
		},
		poller, presence, nil,
		WithPoster(poster.post),
		WithAfterFunc(clock.after),
	)
	return n, presence, clock, poster, poller
}

// recordJoin / recordLeave mirror what hub.Writer does before calling
// OnHumanJoin / OnHumanLeave. Slot is arbitrary; only the IsBot flag
// matters for HumanCount.
func recordJoin(p *hub.Presence, serverID int64, clientNum int, guid string) {
	p.RecordJoin(serverID, clientNum, hub.PresenceEntry{GUID: guid, IsBot: false})
}

func recordLeave(p *hub.Presence, serverID int64, clientNum int, guid string) {
	p.RecordLeave(serverID, clientNum, guid)
}

// 0→1 schedules the going-active timer (delayed) — no immediate
// POST. After the timer fires, exactly one going-active embed lands.
// A subsequent 1→2 join doesn't fire another.
func TestNotifier_FirstHumanFiresActiveAfterDelay(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	// The going-active fire is now timer-gated; not immediate.
	time.Sleep(20 * time.Millisecond)
	if got := len(poster.embeds); got != 0 {
		t.Fatalf("active should not fire before delay elapses, got %d (%v)", got, poster.titles())
	}
	if got := clock.pendingLen(); got != 1 {
		t.Fatalf("expected 1 pending active timer, got %d", got)
	}

	clock.fireAll()
	poster.waitFor(t, 1)

	// Subsequent join 1→2 must not schedule or fire.
	recordJoin(presence, serverID, 1, "guid-B")
	n.OnHumanJoin(serverID)
	time.Sleep(20 * time.Millisecond)

	if got := len(poster.embeds); got != 1 {
		t.Fatalf("want 1 fire on 0→1 only, got %d (%v)", got, poster.titles())
	}
	if want := "🟢  test / alpha is active"; poster.embeds[0].Title != want {
		t.Errorf("title: got %q, want %q", poster.embeds[0].Title, want)
	}
}

// Connect-and-drop inside the going-active delay: cancel the pending
// active fire. Nothing posted on either side — Discord never heard
// about this connection.
func TestNotifier_DropInsideActiveDelayCancels(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	if got := clock.pendingLen(); got != 1 {
		t.Fatalf("expected 1 pending active timer, got %d", got)
	}

	// Drop before the active-delay elapses.
	recordLeave(presence, serverID, 0, "guid-A")
	n.OnHumanLeave(serverID)

	// Pending active should be cancelled; no inactive timer should
	// have been scheduled (we never told Discord they were active).
	if got := clock.pendingLen(); got != 0 {
		t.Fatalf("active timer should be cancelled and no inactive scheduled, got %d still pending", got)
	}

	// Even if a stale fn was queued, fireAll() skips stopped ones.
	clock.fireAll()
	time.Sleep(20 * time.Millisecond)
	if got := len(poster.embeds); got != 0 {
		t.Fatalf("expected no posts on connect-and-drop flap, got %d (%v)", got, poster.titles())
	}
}

// Full active→inactive lifecycle: active timer fires, then last
// human leaves, then inactive timer fires.
func TestNotifier_FullLifecycle(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active timer
	poster.waitFor(t, 1)

	recordLeave(presence, serverID, 0, "guid-A")
	n.OnHumanLeave(serverID)
	if got := clock.pendingLen(); got != 1 {
		t.Fatalf("expected 1 pending inactive timer, got %d", got)
	}

	clock.fireAll() // inactive timer
	poster.waitFor(t, 2)

	titles := poster.titles()
	if titles[0] != "🟢  test / alpha is active" {
		t.Errorf("first embed: %q", titles[0])
	}
	if titles[1] != "⚪  test / alpha is idle" {
		t.Errorf("second embed: %q", titles[1])
	}
}

// 1→0 then 0→1 inside the inactive debounce window cancels the
// pending inactive — Discord never sees the flap.
func TestNotifier_RejoinInsideInactiveDebounceCancels(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active fires
	poster.waitFor(t, 1)

	recordLeave(presence, serverID, 0, "guid-A")
	n.OnHumanLeave(serverID)

	// Rejoin inside the inactive-debounce window: cancel timer, no
	// fire on either side.
	recordJoin(presence, serverID, 1, "guid-A")
	n.OnHumanJoin(serverID)
	time.Sleep(20 * time.Millisecond)
	if got := len(poster.embeds); got != 1 {
		t.Fatalf("expected only the initial active fire, got %d (%v)", got, poster.titles())
	}

	clock.fireAll()
	time.Sleep(20 * time.Millisecond)
	if got := len(poster.embeds); got != 1 {
		t.Fatalf("cancelled inactive timer should not have fired, got %d (%v)", got, poster.titles())
	}
}

// Busy-at-boot servers (humans pre-loaded via PresenceSnapshot,
// without an OnHumanJoin) must not refire "going active" on the
// next join. The count==1 clause carries this — count goes 1→2,
// 2→3, etc, never reaching 1 in a way that would fire.
func TestNotifier_BusyAtBootDoesNotRefireActive(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")

	recordJoin(presence, serverID, 1, "guid-B")
	n.OnHumanJoin(serverID)
	time.Sleep(20 * time.Millisecond)

	if got := clock.pendingLen(); got != 0 {
		t.Fatalf("busy-at-boot join should not schedule a timer, got %d pending", got)
	}
	if got := len(poster.embeds); got != 0 {
		t.Fatalf("busy-at-boot server should not refire on subsequent joins, got %d (%v)", got, poster.titles())
	}
}

// Busy-at-boot drain: snapshot pre-loaded humans, they all leave,
// the inactive fire must still happen even though we never saw an
// OnHumanJoin. Covers the "!= declaredInactive" gate.
func TestNotifier_BusyAtBootDrainFiresInactive(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	recordJoin(presence, serverID, 1, "guid-B")

	recordLeave(presence, serverID, 0, "guid-A")
	n.OnHumanLeave(serverID)
	if got := clock.pendingLen(); got != 0 {
		t.Fatalf("intermediate leave should not schedule a timer, got %d pending", got)
	}

	recordLeave(presence, serverID, 1, "guid-B")
	n.OnHumanLeave(serverID)
	if got := clock.pendingLen(); got != 1 {
		t.Fatalf("final leave should schedule exactly one timer, got %d", got)
	}

	clock.fireAll()
	poster.waitFor(t, 1)

	if title := poster.embeds[0].Title; title != "⚪  test / alpha is idle" {
		t.Errorf("expected going-inactive fire, got %q", title)
	}
}

// Bot-only churn never reaches the notifier because the writer's
// handler filters bots, but if it did somehow (synthetic test of
// the count-based gate), HumanCount stays at 0 so no fire.
func TestNotifier_BotJoinDoesNotFire(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	presence.RecordJoin(serverID, 0, hub.PresenceEntry{GUID: "BOT:sarge", IsBot: true})
	n.OnHumanJoin(serverID)
	time.Sleep(20 * time.Millisecond)

	if got := clock.pendingLen(); got != 0 {
		t.Fatalf("bot join should not schedule a timer, got %d pending", got)
	}
	if got := len(poster.embeds); got != 0 {
		t.Fatalf("bot join should not trigger an active fire, got %d (%v)", got, poster.titles())
	}
}

// Server with no poller status logs a skip and posts nothing —
// defensive against the brief window after notifier startup before
// the first UDP poll lands.
func TestNotifier_NoStatusSkipsFire(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, poller := setup(t, serverID)
	delete(poller.byID, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll()
	time.Sleep(20 * time.Millisecond)

	if got := len(poster.embeds); got != 0 {
		t.Fatalf("missing status should suppress fire, got %d (%v)", got, poster.titles())
	}
}
