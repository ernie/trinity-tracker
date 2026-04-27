package natsbus

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// BufferedCapacity is the default in-memory ring cap. Matches the
// design spec's "default 10 000 events". At ~500B per envelope this
// is ~5 MB resident.
const BufferedCapacity = 10_000

// BufferedRetryBackoff is how long the drain loop waits after a
// failed publish before retrying. A fresh NATS reconnect inside
// nats.go's default settings usually happens within hundreds of ms,
// so 250ms is a reasonable steady-state.
const BufferedRetryBackoff = 250 * time.Millisecond

// BufferedPublisher decouples the collector's log-event path from
// NATS availability. Publish never blocks on NATS: events are
// enqueued into an in-memory ring and drained by a background
// goroutine that retries the underlying Publisher on failure.
//
// Overflow drops the oldest queued event and increments the dropped
// counter — recent events are more valuable than ancient ones.
//
// Disk-spill overflow (the design spec's buffer.jsonl) is deferred:
// a future phase adds it once the 10k in-memory cap proves
// insufficient in practice.
type BufferedPublisher struct {
	inner *Publisher
	ch    chan domain.FactEvent

	dropped atomic.Uint64

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewBufferedPublisher wraps the given Publisher in a buffered
// asynchronous queue. capacity <= 0 falls back to BufferedCapacity.
func NewBufferedPublisher(inner *Publisher, capacity int) *BufferedPublisher {
	if capacity <= 0 {
		capacity = BufferedCapacity
	}
	return &BufferedPublisher{
		inner:  inner,
		ch:     make(chan domain.FactEvent, capacity),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start launches the drain goroutine. Must be called before Publish
// is used; ctx cancellation also shuts down the drain.
func (b *BufferedPublisher) Start(ctx context.Context) {
	go b.run(ctx)
}

// Stop drains the current queue and tears down the goroutine. The
// drain attempts each pending event exactly once; undelivered events
// are dropped (logged) so the caller can shut down in bounded time.
func (b *BufferedPublisher) Stop() {
	b.stopOnce.Do(func() { close(b.stopCh) })
	<-b.doneCh
}

// Publish enqueues the event and returns immediately. Always
// nil-returning: overflow drops the oldest queued event, counted in
// Dropped.
func (b *BufferedPublisher) Publish(e domain.FactEvent) error {
	select {
	case b.ch <- e:
		return nil
	default:
	}
	b.dropped.Add(1)
	// Drop-oldest: try to discard one queued event, then push.
	select {
	case <-b.ch:
	default:
	}
	// The channel may still be full if a concurrent drain already
	// consumed; loop-send guards against that rare race.
	for {
		select {
		case b.ch <- e:
			return nil
		case <-b.ch:
			// Channel still saturated; drop one more and retry.
			b.dropped.Add(1)
		case <-b.stopCh:
			return nil
		}
	}
}

// Dropped returns the running count of events discarded due to
// overflow. Exported for metrics.
func (b *BufferedPublisher) Dropped() uint64 { return b.dropped.Load() }

func (b *BufferedPublisher) run(ctx context.Context) {
	defer close(b.doneCh)
	for {
		select {
		case <-b.stopCh:
			b.drainOnce(ctx)
			return
		case <-ctx.Done():
			b.drainOnce(ctx)
			return
		case e := <-b.ch:
			b.publishWithRetry(ctx, e)
		}
	}
}

// publishWithRetry retries the inner publisher until success, the
// stopCh fires, or ctx cancels. NATS reconnects happen inside nats.go,
// so a persistent error means either permanent misconfiguration or
// a shutdown race — the backoff-based retry handles both safely.
func (b *BufferedPublisher) publishWithRetry(ctx context.Context, e domain.FactEvent) {
	for {
		if err := b.inner.Publish(e); err == nil {
			return
		} else {
			log.Printf("natsbus.BufferedPublisher: publish failed (will retry): %v", err)
		}
		select {
		case <-b.stopCh:
			return
		case <-ctx.Done():
			return
		case <-time.After(BufferedRetryBackoff):
		}
	}
}

// drainOnce attempts one final pass over pending events on shutdown,
// using a short deadline so the process can exit in bounded time.
func (b *BufferedPublisher) drainOnce(ctx context.Context) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		select {
		case e := <-b.ch:
			if time.Now().After(deadline) {
				log.Printf("natsbus.BufferedPublisher: shutdown drain timed out; %d+ events discarded", 1+len(b.ch))
				return
			}
			if err := b.inner.Publish(e); err != nil {
				log.Printf("natsbus.BufferedPublisher: shutdown drain publish failed: %v", err)
				return
			}
		default:
			return
		}
	}
}
