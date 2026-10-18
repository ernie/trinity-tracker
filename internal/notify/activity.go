// Package notify posts Discord notifications. The activity notifier
// turns the hub's human join and leave callbacks into "active" and
// "idle" embeds, delayed on both sides so a quick connect and drop
// doesn't spam the channel.
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

// PollerStatus is the part of *hub.RemotePoller the notifier reads to
// build embeds; tests substitute a stub.
type PollerStatus interface {
	GetServerStatus(serverID int64) *domain.ServerStatus
}

// ActivityConfig holds what NewNotifier needs; cmd/trinity builds it
// from *config.ActivityConfig so this package needn't import config.
type ActivityConfig struct {
	WebhookURL       string
	PublicURL        string
	ActiveDelay      time.Duration // 0 → DefaultActiveDelay
	InactiveDebounce time.Duration // 0 → DefaultInactiveDebounce
}

// DefaultActiveDelay is the wait between the first human joining and
// the "active" post. The poller (every 5s by default) may not have the
// new player yet when the join arrives, and someone who connects and
// drops within this window (a missed map load, a port scan) isn't worth
// announcing. 30s covers about six polls and a real player's map load
// and warmup.
const DefaultActiveDelay = 30 * time.Second

// DefaultInactiveDebounce is the wait between the last human leaving
// and the "idle" post, long enough to absorb a typical reconnect.
const DefaultInactiveDebounce = 60 * time.Second

// declaredState is what the notifier last told Discord about a server,
// or unknown before anything fired in this process. It keeps the same
// edge from being announced twice.
type declaredState int

const (
	declaredUnknown declaredState = iota
	declaredActive
	declaredInactive
)

// Notifier tracks each server's activity and posts a Discord embed
// when a server goes from zero humans to one or back, delayed on both
// sides. While a server is active, joins and leaves edit the original
// "active" embed, so the channel shows one live roster per session.
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

// Stopper is the part of *time.Timer the notifier uses, so tests can
// substitute a timer they control.
type Stopper interface {
	Stop() bool
}

// serverState holds the declared state and any pending posts.
// pendingActive and pendingInactive never coexist: each event cancels
// the opposite one. pendingRefresh batches roster changes into edits of
// the active message while the server is active.
//
// activeMessageID is the message the "active" post created; the
// notifier edits it until the "idle" post fires or Discord reports it
// deleted.
type serverState struct {
	declared        declaredState
	pendingActive   Stopper
	pendingInactive Stopper
	pendingRefresh  Stopper
	activeMessageID string
}

// Option customizes a Notifier, mainly for tests.
type Option func(*Notifier)

// WithLogger overrides the default log.Default() logger.
func WithLogger(l *log.Logger) Option {
	return func(n *Notifier) { n.log = l }
}

// WithPoster replaces the Discord post function, which returns the new
// message's id as PostWebhookForID does. Tests that don't care about the
// id can return any non-empty string.
func WithPoster(f func(ctx context.Context, url string, embed discord.Embed) (string, error)) Option {
	return func(n *Notifier) { n.poster = f }
}

// WithEditor replaces the Discord message edit function.
func WithEditor(f func(ctx context.Context, url, messageID string, embed discord.Embed) error) Option {
	return func(n *Notifier) { n.editor = f }
}

// WithAfterFunc replaces time.AfterFunc, so tests can fire timers at
// once.
func WithAfterFunc(f func(time.Duration, func()) Stopper) Option {
	return func(n *Notifier) { n.afterFunc = f }
}

// NewNotifier builds a Notifier over the hub's presence map and poller
// cache. mapMeta maps short map names to the full names in maps.json;
// with nil, embeds show short names only.
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

// OnHumanJoin reports a human joining serverID. It cancels a pending
// "idle" post, since the player came back, and when this is the first
// human it schedules the delayed "active" post.
func (n *Notifier) OnHumanJoin(serverID int64) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	st := n.ensureState(serverID)

	if st.pendingInactive != nil {
		st.pendingInactive.Stop()
		st.pendingInactive = nil
	}

	// The writer's RecordJoin has already updated presence, so a count
	// of 1 means this was the first human.
	count := n.presence.HumanCount(serverID)
	if count == 1 && st.declared != declaredActive {
		if st.pendingActive == nil {
			st.pendingActive = n.afterFunc(n.activeDelay, func() { n.fireActive(serverID) })
		}
		return
	}
	n.maybeScheduleRefresh(st, serverID)
}

// OnHumanLeave reports a human leaving serverID. When the last human
// leaves, it cancels a pending "active" post (nothing is announced
// either way) or schedules the delayed "idle" post.
//
// The idle post is gated on "not already idle" so it also fires for an
// unknown state: after a hub restart, the collector sends
// PresenceSnapshot for players already connected, so the state stays
// unknown until they leave.
func (n *Notifier) OnHumanLeave(serverID int64) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	st := n.ensureState(serverID)
	count := n.presence.HumanCount(serverID)
	if count != 0 {
		n.maybeScheduleRefresh(st, serverID)
		return
	}

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

// OnMatchStart reports a new match on serverID. On an active server it
// schedules a refresh so the embed picks up the new map and scores once
// the poller catches up; otherwise the next "active" post covers it.
func (n *Notifier) OnMatchStart(serverID int64) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	st := n.ensureState(serverID)
	n.maybeScheduleRefresh(st, serverID)
}

// maybeScheduleRefresh schedules an edit of the active embed. The
// caller holds n.mu. A pending "idle" post takes precedence, and an
// already scheduled refresh is left alone so the wait stays bounded.
// The delay is activeDelay, so a mid-session joiner has to stay as long
// as the first player did before appearing.
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
	// The player may have left after the timer started; a Stop from
	// OnHumanLeave can lose the race with this goroutine.
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
	// The idle post is a new message, so the channel keeps the original
	// "active" alert.
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

// fireRefresh edits the active embed with the current poller status.
// It does nothing if the server stopped being active meanwhile. If
// someone deleted the message, it clears the id so later refreshes are
// skipped and the next session posts a new message.
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
		n.mu.Lock()
		if st, ok := n.states[serverID]; ok && st.activeMessageID == messageID {
			st.activeMessageID = ""
		}
		n.mu.Unlock()
		return
	}
	n.log.Printf("notify: PATCH webhook: %v", err)
}
