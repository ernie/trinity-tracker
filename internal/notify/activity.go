// Package notify ships event-driven Discord notifications. The
// activity notifier turns hub-level human join/leave callbacks into
// "server is active" / "server is idle" embeds, debounced on both
// sides so quick connect-then-drop flaps don't spam the channel.
package notify

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/discord"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
)

// PollerStatus is the read-only slice of *hub.RemotePoller the
// notifier consults to build embed bodies. Declared as an interface
// so tests can substitute a stub without spinning up real UDP polls.
type PollerStatus interface {
	GetServerStatus(serverID int64) *domain.ServerStatus
}

// ActivityConfig is the lightweight transport NewNotifier consumes.
// cmd/trinity translates *config.ActivityConfig into this so the
// notify package doesn't need to import config.
type ActivityConfig struct {
	WebhookURL       string
	PublicURL        string
	ActiveDelay      time.Duration // 0 → DefaultActiveDelay
	InactiveDebounce time.Duration // 0 → DefaultInactiveDebounce
}

// DefaultActiveDelay is the wait between "first human joined" and
// the going-active POST. Two reasons it exists:
//
//   - The poller (default 5s interval) reads server state via UDP
//     getstatus and may not yet have the new player when the join
//     event arrives. Without a delay the embed's roster shows pre-
//     join data (the bug the first deploy surfaced).
//   - A real player who actually intends to play stays connected
//     for more than a few seconds. Anyone who connects and drops
//     inside this window was almost certainly a missed map load,
//     a long warmup before pressing fire, or a port scan — no
//     announcement worth posting.
//
// 30s is a comfortable cover for both: ~6 poll cycles and well
// past Q3's map-load + warmup-fidget window for a real player.
const DefaultActiveDelay = 30 * time.Second

// DefaultInactiveDebounce is the wait between "last human left"
// and the going-inactive POST. Long enough to absorb typical
// reconnect lag.
const DefaultInactiveDebounce = 60 * time.Second

// declaredState is what the notifier last told Discord (or the
// initial "we don't know yet" before any fire has run for this
// server in the current process). Gates the fire predicates so we
// don't announce the same edge twice.
type declaredState int

const (
	declaredUnknown declaredState = iota
	declaredActive
	declaredInactive
)

// Notifier tracks per-server activity state and posts a Discord
// embed each time a server crosses the 0↔1 human-player threshold,
// with symmetric debouncing on both sides. While a server is
// declared active, subsequent joins/leaves trigger an in-place
// PATCH of the original "going active" embed so the channel shows
// a live roster rather than accumulating new messages per player.
type Notifier struct {
	webhookURL string
	publicURL  string
	poller     PollerStatus
	presence   *hub.Presence
	mapMeta    map[string]string

	activeDelay      time.Duration
	inactiveDebounce time.Duration
	log              *log.Logger
	poster           func(ctx context.Context, url string, embed discord.Embed) (string, error)
	editor           func(ctx context.Context, url, messageID string, embed discord.Embed) error
	afterFunc        func(time.Duration, func()) Stopper

	mu     sync.Mutex
	states map[int64]*serverState
}

// Stopper is the subset of *time.Timer the notifier uses. Defined
// here so tests can substitute a controllable timer without spinning
// a wall-clock goroutine. *time.Timer satisfies it directly.
type Stopper interface {
	Stop() bool
}

// serverState carries the declared edge plus any pending fire.
// pendingActive / pendingInactive are mutually exclusive — the
// cancel-on-opposite-event rule enforces that. pendingRefresh runs
// independently while the server is declared active to coalesce
// roster changes into PATCH edits of the existing message.
//
// activeMessageID is the Discord message id returned by the
// initial going-active POST. While non-empty the notifier edits
// that message; cleared on going-inactive fire (next active starts
// a fresh message) or on a 404 from Discord (someone deleted the
// message).
type serverState struct {
	declared        declaredState
	pendingActive   Stopper
	pendingInactive Stopper
	pendingRefresh  Stopper
	activeMessageID string
}

// Option customizes a Notifier — chiefly for tests.
type Option func(*Notifier)

// WithLogger overrides the default log.Default() logger.
func WithLogger(l *log.Logger) Option {
	return func(n *Notifier) { n.log = l }
}

// WithPoster injects an alternate Discord-post function. Tests
// use this to capture POSTs without hitting the network. The
// function returns the new message's id (as PostWebhookForID
// does); pass any non-empty string for tests that don't care.
func WithPoster(f func(ctx context.Context, url string, embed discord.Embed) (string, error)) Option {
	return func(n *Notifier) { n.poster = f }
}

// WithEditor injects an alternate Discord-edit function. Tests
// use this to capture refresh PATCHes without hitting the network.
func WithEditor(f func(ctx context.Context, url, messageID string, embed discord.Embed) error) Option {
	return func(n *Notifier) { n.editor = f }
}

// WithAfterFunc injects an alternate time.AfterFunc. Tests fire
// scheduled timers immediately rather than waiting wall-clock
// seconds.
func WithAfterFunc(f func(time.Duration, func()) Stopper) Option {
	return func(n *Notifier) { n.afterFunc = f }
}

// NewNotifier builds a Notifier wired to the runtime presence map
// and poller cache. mapMeta is the short-id → display-name table
// loaded from maps.json; pass nil to render short ids only.
func NewNotifier(cfg ActivityConfig, poller PollerStatus, presence *hub.Presence, mapMeta map[string]string, opts ...Option) *Notifier {
	n := &Notifier{
		webhookURL:       cfg.WebhookURL,
		publicURL:        strings.TrimSuffix(cfg.PublicURL, "/"),
		poller:           poller,
		presence:         presence,
		mapMeta:          mapMeta,
		activeDelay:      cfg.ActiveDelay,
		inactiveDebounce: cfg.InactiveDebounce,
		log:              log.Default(),
		poster:           discord.PostWebhookForID,
		editor:           discord.EditWebhookMessage,
		afterFunc:        func(d time.Duration, fn func()) Stopper { return time.AfterFunc(d, fn) },
		states:           make(map[int64]*serverState),
	}
	if n.activeDelay <= 0 {
		n.activeDelay = DefaultActiveDelay
	}
	if n.inactiveDebounce <= 0 {
		n.inactiveDebounce = DefaultInactiveDebounce
	}
	for _, opt := range opts {
		opt(n)
	}
	return n
}

// OnHumanJoin reports a human-player join on serverID.
//
//   - If a pending going-inactive timer was waiting (server was
//     active, last human left, debounce in progress), cancel it —
//     the rejoin saves the channel from a flap pair.
//   - If this push the human count from 0 to 1 and the server
//     isn't already declared active, schedule a delayed going-
//     active POST. The delay gives the poller time to capture the
//     new player on its next UDP probe and rules out connect-and-
//     drop noise.
func (n *Notifier) OnHumanJoin(serverID int64) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	st := n.ensureState(serverID)

	// Cancel any pending going-inactive — they came back.
	if st.pendingInactive != nil {
		st.pendingInactive.Stop()
		st.pendingInactive = nil
	}

	// HumanCount runs *after* the writer's RecordJoin updates
	// presence, so count==1 means this caller was the first human.
	count := n.presence.HumanCount(serverID)
	if count == 1 && st.declared != declaredActive {
		if st.pendingActive == nil {
			st.pendingActive = n.afterFunc(n.activeDelay, func() { n.fireActive(serverID) })
		}
		return
	}
	// We're already declared active (additional player joining) or
	// still climbing toward the first commitment (count > 1 but
	// pendingActive is set). In the former, schedule a refresh; in
	// the latter, maybeScheduleRefresh is a no-op.
	n.maybeScheduleRefresh(st, serverID)
}

// OnHumanLeave reports a human-player leave on serverID.
//
//   - If a pending going-active timer was waiting (first human
//     joined, delay in progress) and the count just hit zero,
//     cancel it — they connected and dropped before the
//     announcement, so we never tell Discord anything.
//   - Otherwise, if count hit zero and the server isn't already
//     declared inactive, schedule a debounced going-inactive POST.
//
// The "!= declaredInactive" gate (rather than "== declaredActive")
// covers the busy-at-boot drain case: after a hub restart, the
// collector publishes PresenceSnapshot rather than PlayerJoin for
// already-connected humans, so declared stays declaredUnknown.
// When those humans eventually leave, declaredUnknown still passes
// the gate so the inactive fire happens once.
func (n *Notifier) OnHumanLeave(serverID int64) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	st := n.ensureState(serverID)
	count := n.presence.HumanCount(serverID)
	if count != 0 {
		// Someone left but the server is still busy. Refresh the
		// live embed so the roster reflects the new count.
		n.maybeScheduleRefresh(st, serverID)
		return
	}

	// Cancel a pending going-active — they left before we
	// announced. Nothing to post on either side.
	if st.pendingActive != nil {
		st.pendingActive.Stop()
		st.pendingActive = nil
		return
	}

	if st.declared == declaredInactive {
		return
	}
	if st.pendingInactive != nil {
		st.pendingInactive.Stop()
	}
	st.pendingInactive = n.afterFunc(n.inactiveDebounce, func() { n.fireInactive(serverID) })
}

// OnMatchStart reports that a new match has begun on serverID
// (new map or new match on the same map). If the server is
// declared active, schedule a refresh so the embed picks up the
// new map name and reset scores once the UDP poller has caught
// up. Outside the active state this is a no-op — pre-active
// match-starts get rolled into the going-active POST when it
// fires, and inactive servers have no embed to refresh.
func (n *Notifier) OnMatchStart(serverID int64) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	st := n.ensureState(serverID)
	n.maybeScheduleRefresh(st, serverID)
}

// maybeScheduleRefresh queues a debounced PATCH of the active
// embed. Caller holds n.mu. No-op when there is no message to
// refresh, when going-inactive is pending (the inactive fire would
// supersede a refresh), or when a refresh is already scheduled —
// leaving the existing timer in place bounds the wait. The delay
// reuses activeDelay so a player who joins mid-session has to
// commit to staying before they appear in the embed, matching the
// "is this a real player" reasoning the initial going-active POST
// already applies.
func (n *Notifier) maybeScheduleRefresh(st *serverState, serverID int64) {
	if st.declared != declaredActive || st.activeMessageID == "" {
		return
	}
	if st.pendingInactive != nil {
		return
	}
	if st.pendingRefresh != nil {
		return
	}
	st.pendingRefresh = n.afterFunc(n.activeDelay, func() { n.fireRefresh(serverID) })
}

func (n *Notifier) ensureState(serverID int64) *serverState {
	st, ok := n.states[serverID]
	if !ok {
		st = &serverState{declared: declaredUnknown}
		n.states[serverID] = st
	}
	return st
}

func (n *Notifier) fireActive(serverID int64) {
	// Defensive: the joining human may have left between the timer's
	// scheduling and its firing (e.g., OnHumanLeave cancelled but
	// lost the race against the goroutine starting). Re-check.
	n.mu.Lock()
	st, ok := n.states[serverID]
	if ok {
		st.pendingActive = nil
	}
	count := n.presence.HumanCount(serverID)
	if !ok || count == 0 || st.declared == declaredActive {
		n.mu.Unlock()
		return
	}
	st.declared = declaredActive
	n.mu.Unlock()

	status := n.poller.GetServerStatus(serverID)
	if status == nil {
		n.log.Printf("notify: going-active fire skipped, no poller status for server %d", serverID)
		return
	}
	if n.webhookURL == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	messageID, err := n.poster(ctx, n.webhookURL, buildActiveEmbed(status, n.publicURL, n.mapMeta))
	if err != nil {
		n.log.Printf("notify: POST webhook: %v", err)
		return
	}
	// Stash the message id so subsequent join/leave events can
	// PATCH it instead of posting fresh messages.
	n.mu.Lock()
	if st, ok := n.states[serverID]; ok {
		st.activeMessageID = messageID
	}
	n.mu.Unlock()
}

func (n *Notifier) fireInactive(serverID int64) {
	n.mu.Lock()
	st, ok := n.states[serverID]
	if ok {
		st.pendingInactive = nil
	}
	count := n.presence.HumanCount(serverID)
	if !ok || count > 0 || st.declared == declaredInactive {
		n.mu.Unlock()
		return
	}
	st.declared = declaredInactive
	// Clear the active message id and any pending refresh — the
	// going-inactive POST is a new message, not an edit of the
	// active one (editing "Server is active" into "Server is idle"
	// would erase the original alert from the channel).
	st.activeMessageID = ""
	if st.pendingRefresh != nil {
		st.pendingRefresh.Stop()
		st.pendingRefresh = nil
	}
	n.mu.Unlock()

	status := n.poller.GetServerStatus(serverID)
	if status == nil {
		n.log.Printf("notify: going-inactive fire skipped, no poller status for server %d", serverID)
		return
	}
	if n.webhookURL == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := n.poster(ctx, n.webhookURL, buildInactiveEmbed(status, n.publicURL, n.mapMeta)); err != nil {
		n.log.Printf("notify: POST webhook: %v", err)
	}
}

// fireRefresh PATCHes the active embed with current poller status.
// Skips silently if the server transitioned out of active in the
// meantime, or if Discord reports the message is gone (someone
// deleted it from the channel) — in that case we clear the id so
// future refresh attempts don't keep failing against a tombstone.
func (n *Notifier) fireRefresh(serverID int64) {
	n.mu.Lock()
	st, ok := n.states[serverID]
	if ok {
		st.pendingRefresh = nil
	}
	if !ok || st.declared != declaredActive || st.activeMessageID == "" {
		n.mu.Unlock()
		return
	}
	count := n.presence.HumanCount(serverID)
	messageID := st.activeMessageID
	n.mu.Unlock()
	if count == 0 {
		// Server emptied between scheduling and firing — the
		// inactive flow will handle the message; skip the refresh.
		return
	}

	status := n.poller.GetServerStatus(serverID)
	if status == nil {
		n.log.Printf("notify: refresh skipped, no poller status for server %d", serverID)
		return
	}
	if n.webhookURL == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := n.editor(ctx, n.webhookURL, messageID, buildActiveEmbed(status, n.publicURL, n.mapMeta))
	if err == nil {
		return
	}
	if errors.Is(err, discord.ErrMessageNotFound) {
		// Discord lost the message — clear our id so we don't
		// keep PATCHing a tombstone. Future events will see no id
		// and skip refreshes; the next 0→1 transition posts fresh.
		n.mu.Lock()
		if st, ok := n.states[serverID]; ok && st.activeMessageID == messageID {
			st.activeMessageID = ""
		}
		n.mu.Unlock()
		return
	}
	n.log.Printf("notify: PATCH webhook: %v", err)
}
