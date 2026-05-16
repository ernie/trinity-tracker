package notify

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/discord"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
)

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

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

// capturePoster records every webhook POST and PATCH so the test
// can assert on fire counts and order. Posts get sequential ids
// so the editor can match by message id.
type capturePoster struct {
	mu          sync.Mutex
	embeds      []discord.Embed
	nextID      int
	edits       []editRecord
	editErr     error // returned by next edit call, if non-nil
}

type editRecord struct {
	messageID string
	embed     discord.Embed
}

func (c *capturePoster) post(_ context.Context, _ string, embed discord.Embed) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	c.embeds = append(c.embeds, embed)
	return fmt.Sprintf("msg-%d", c.nextID), nil
}

func (c *capturePoster) edit(_ context.Context, _, messageID string, embed discord.Embed) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.editErr != nil {
		err := c.editErr
		c.editErr = nil
		return err
	}
	c.edits = append(c.edits, editRecord{messageID: messageID, embed: embed})
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

func (c *capturePoster) editCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.edits)
}

func (c *capturePoster) lastEdit() (editRecord, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.edits) == 0 {
		return editRecord{}, false
	}
	return c.edits[len(c.edits)-1], true
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

func (c *capturePoster) waitForEdits(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c.editCount() >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d edit(s), have %d", n, c.editCount())
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
		WithEditor(poster.edit),
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

// After going-active has fired and recorded a message id, a
// second human joining schedules a refresh that PATCHes the same
// message id with an updated embed. No new POST.
func TestNotifier_AdditionalJoinSchedulesRefresh(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active timer
	poster.waitFor(t, 1)

	// Second player joins while declared=active.
	recordJoin(presence, serverID, 1, "guid-B")
	n.OnHumanJoin(serverID)
	if got := clock.pendingLen(); got != 1 {
		t.Fatalf("expected 1 pending refresh timer, got %d", got)
	}

	clock.fireAll() // refresh timer
	poster.waitForEdits(t, 1)

	if got := len(poster.embeds); got != 1 {
		t.Errorf("refresh must edit, not post a new message; got %d posts", got)
	}
	last, _ := poster.lastEdit()
	if last.messageID != "msg-1" {
		t.Errorf("edit message id: got %q, want msg-1", last.messageID)
	}
	if last.embed.Title != "🟢  test / alpha is active" {
		t.Errorf("edit title: got %q", last.embed.Title)
	}
}

// A leave that keeps count > 0 also refreshes the roster so the
// embed reflects the new player list. Same commitment window as
// joins (no twitchy updates from a player who immediately rejoins).
func TestNotifier_LeaveWithRemainingSchedulesRefresh(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active
	poster.waitFor(t, 1)

	recordJoin(presence, serverID, 1, "guid-B")
	n.OnHumanJoin(serverID)
	clock.fireAll() // first refresh
	poster.waitForEdits(t, 1)

	// One player leaves; another remains. Refresh should fire.
	recordLeave(presence, serverID, 0, "guid-A")
	n.OnHumanLeave(serverID)
	if got := clock.pendingLen(); got != 1 {
		t.Fatalf("expected refresh scheduled, got %d pending", got)
	}
	clock.fireAll()
	poster.waitForEdits(t, 2)

	if got := len(poster.embeds); got != 1 {
		t.Errorf("expected no new POST, got %d", got)
	}
}

// While the going-inactive debounce is pending we don't schedule
// a refresh — the inactive fire would supersede it. Avoids a
// "Server is active with 0 players" frame between the leave and
// the inactive fire.
func TestNotifier_NoRefreshWhileInactivePending(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active
	poster.waitFor(t, 1)

	recordLeave(presence, serverID, 0, "guid-A")
	n.OnHumanLeave(serverID)
	// Pending should be exactly the inactive debounce — no refresh.
	if got := clock.pendingLen(); got != 1 {
		t.Fatalf("expected only inactive timer, got %d pending", got)
	}

	// Even if a stray rejoin happens, cancelling inactive shouldn't
	// schedule a refresh until count > 0 *and* declared==active.
	// (After the rejoin, declared remains active, so a refresh is
	// allowed and expected — this checks that the inactive-pending
	// case alone doesn't schedule one.)
}

// Going-inactive clears activeMessageID and the next 0→1 starts a
// fresh POST. Without this the second "active" would PATCH the
// previous (now silent) embed.
func TestNotifier_InactiveClearsActiveMessageID(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active
	poster.waitFor(t, 1)

	recordLeave(presence, serverID, 0, "guid-A")
	n.OnHumanLeave(serverID)
	clock.fireAll() // inactive
	poster.waitFor(t, 2)

	// Next 0→1 transition should produce a third POST, not a PATCH.
	recordJoin(presence, serverID, 1, "guid-B")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active
	poster.waitFor(t, 3)

	if got := poster.editCount(); got != 0 {
		t.Errorf("expected zero edits across lifecycle, got %d", got)
	}
}

// OnMatchStart while active schedules a refresh so a map change
// (or a new match on the same map with reset scores) updates the
// embed. The 30s window lets the UDP poller observe the new
// mapname cvar before we PATCH.
func TestNotifier_MatchStartRefreshesWhileActive(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, poller := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active
	poster.waitFor(t, 1)

	// Update poller status to simulate the new map landing in the
	// next UDP poll cycle, then trigger OnMatchStart.
	poller.byID[serverID].Map = "q3dm6"
	n.OnMatchStart(serverID)
	if got := clock.pendingLen(); got != 1 {
		t.Fatalf("expected refresh scheduled after match start, got %d pending", got)
	}

	clock.fireAll() // refresh
	poster.waitForEdits(t, 1)
	last, _ := poster.lastEdit()
	if !contains(last.embed.Description, "q3dm6") {
		t.Errorf("refresh embed should mention new map q3dm6, got %q", last.embed.Description)
	}
}

// OnMatchStart on an inactive or pre-announce server is a no-op:
// nothing to refresh.
func TestNotifier_MatchStartIgnoredWhenInactive(t *testing.T) {
	const serverID = int64(7)
	n, _, clock, _, _ := setup(t, serverID)

	n.OnMatchStart(serverID)
	if got := clock.pendingLen(); got != 0 {
		t.Errorf("match start on inactive server should not schedule anything, got %d pending", got)
	}
}

// Discord 404 on PATCH means someone deleted the message in the
// channel. The notifier clears its cached id so future events
// don't keep failing against a tombstone.
func TestNotifier_EditNotFoundClearsMessageID(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active
	poster.waitFor(t, 1)

	// Next refresh hits a 404.
	poster.editErr = discord.ErrMessageNotFound
	recordJoin(presence, serverID, 1, "guid-B")
	n.OnHumanJoin(serverID)
	clock.fireAll() // refresh (will fail with 404)
	time.Sleep(20 * time.Millisecond)

	// A follow-up event must NOT schedule another refresh — the id
	// is gone so maybeScheduleRefresh has nothing to refresh.
	recordJoin(presence, serverID, 2, "guid-C")
	n.OnHumanJoin(serverID)
	if got := clock.pendingLen(); got != 0 {
		t.Errorf("after 404, expected no pending refresh, got %d", got)
	}
}
