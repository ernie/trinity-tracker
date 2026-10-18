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

// fakePoller returns a fixed status per server; embed_test.go covers
// the embed bodies.
type fakePoller struct {
	byID map[int64]*domain.ServerStatus
}

func (f *fakePoller) GetServerStatus(id int64) *domain.ServerStatus {
	if f == nil {
		return nil
	}
	return f.byID[id]
}

// fakeTimer fires only when the test calls fireAll.
type fakeTimer struct {
	fn      func()
	stopped bool
}

func (t *fakeTimer) Stop() bool {
	prev := t.stopped
	t.stopped = true
	return !prev
}

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

// fireAll runs every pending timer that wasn't stopped.
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

// capturePoster records every post and edit. Posts get sequential ids
// so edits can be matched to them.
type capturePoster struct {
	mu      sync.Mutex
	embeds  []discord.Embed
	nextID  int
	edits   []editRecord
	editErr error // returned by next edit call, if non-nil
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

// setup returns a notifier over a hub.Presence the test updates itself,
// as the writer does before each OnHumanJoin or OnHumanLeave.
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

// recordJoin and recordLeave do what hub.Writer does before calling
// OnHumanJoin and OnHumanLeave. Only IsBot matters to HumanCount.
func recordJoin(p *hub.Presence, serverID int64, clientNum int, guid string) {
	p.RecordJoin(serverID, clientNum, hub.PresenceEntry{GUID: guid, IsBot: false})
}

func recordLeave(p *hub.Presence, serverID int64, clientNum int, guid string) {
	p.RecordLeave(serverID, clientNum, guid)
}

func TestNotifier_FirstHumanFiresActiveAfterDelay(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	time.Sleep(20 * time.Millisecond)
	if got := len(poster.embeds); got != 0 {
		t.Fatalf("active should not fire before delay elapses, got %d (%v)", got, poster.titles())
	}
	if got := clock.pendingLen(); got != 1 {
		t.Fatalf("expected 1 pending active timer, got %d", got)
	}

	clock.fireAll()
	poster.waitFor(t, 1)

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

func TestNotifier_DropInsideActiveDelayCancels(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	if got := clock.pendingLen(); got != 1 {
		t.Fatalf("expected 1 pending active timer, got %d", got)
	}

	recordLeave(presence, serverID, 0, "guid-A")
	n.OnHumanLeave(serverID)

	if got := clock.pendingLen(); got != 0 {
		t.Fatalf("active timer should be canceled and no inactive scheduled, got %d still pending", got)
	}

	clock.fireAll()
	time.Sleep(20 * time.Millisecond)
	if got := len(poster.embeds); got != 0 {
		t.Fatalf("expected no posts on connect-and-drop flap, got %d (%v)", got, poster.titles())
	}
}

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

func TestNotifier_RejoinInsideInactiveDebounceCancels(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active fires
	poster.waitFor(t, 1)

	recordLeave(presence, serverID, 0, "guid-A")
	n.OnHumanLeave(serverID)

	recordJoin(presence, serverID, 1, "guid-A")
	n.OnHumanJoin(serverID)
	time.Sleep(20 * time.Millisecond)
	if got := len(poster.embeds); got != 1 {
		t.Fatalf("expected only the initial active fire, got %d (%v)", got, poster.titles())
	}

	clock.fireAll()
	time.Sleep(20 * time.Millisecond)
	if got := len(poster.embeds); got != 1 {
		t.Fatalf("canceled inactive timer should not have fired, got %d (%v)", got, poster.titles())
	}
}

func TestNotifier_BusyAtBootDoesNotRefireActive(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	// Loaded by PresenceSnapshot, so no OnHumanJoin.
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

func TestNotifier_BusyAtBootDrainFiresInactive(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	// Loaded by PresenceSnapshot, so no OnHumanJoin.
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

// The writer filters bots before calling the notifier; this checks the
// count-based gate on its own.
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

// Right after startup the first UDP poll may not have landed yet.
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

func TestNotifier_AdditionalJoinSchedulesRefresh(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active timer
	poster.waitFor(t, 1)

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

func TestNotifier_NoRefreshWhileInactivePending(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active
	poster.waitFor(t, 1)

	recordLeave(presence, serverID, 0, "guid-A")
	n.OnHumanLeave(serverID)
	if got := clock.pendingLen(); got != 1 {
		t.Fatalf("expected only inactive timer, got %d pending", got)
	}
}

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

	recordJoin(presence, serverID, 1, "guid-B")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active
	poster.waitFor(t, 3)

	if got := poster.editCount(); got != 0 {
		t.Errorf("expected zero edits across lifecycle, got %d", got)
	}
}

func TestNotifier_MatchStartRefreshesWhileActive(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, poller := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active
	poster.waitFor(t, 1)

	// The next poll sees the new map.
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

func TestNotifier_MatchStartIgnoredWhenInactive(t *testing.T) {
	const serverID = int64(7)
	n, _, clock, _, _ := setup(t, serverID)

	n.OnMatchStart(serverID)
	if got := clock.pendingLen(); got != 0 {
		t.Errorf("match start on inactive server should not schedule anything, got %d pending", got)
	}
}

func TestNotifier_EditNotFoundClearsMessageID(t *testing.T) {
	const serverID = int64(7)
	n, presence, clock, poster, _ := setup(t, serverID)

	recordJoin(presence, serverID, 0, "guid-A")
	n.OnHumanJoin(serverID)
	clock.fireAll() // active
	poster.waitFor(t, 1)

	poster.editErr = discord.ErrMessageNotFound
	recordJoin(presence, serverID, 1, "guid-B")
	n.OnHumanJoin(serverID)
	clock.fireAll() // refresh, which gets the 404
	time.Sleep(20 * time.Millisecond)

	recordJoin(presence, serverID, 2, "guid-C")
	n.OnHumanJoin(serverID)
	if got := clock.pendingLen(); got != 0 {
		t.Errorf("after 404, expected no pending refresh, got %d", got)
	}
}
