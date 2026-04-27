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

// BufferedHWMFraction is the fraction of capacity at which the
// publisher starts spilling new events to disk instead of pushing
// onto the ring. At 80% we leave the ring some headroom for bursts
// that arrive while the drain goroutine is mid-publish.
const BufferedHWMFraction = 0.8

// BufferedRetryBackoff is how long the drain loop waits after a
// failed publish before retrying. A fresh NATS reconnect inside
// nats.go's default settings usually happens within hundreds of ms,
// so 250ms is a reasonable steady-state.
const BufferedRetryBackoff = 250 * time.Millisecond

// BufferedPublisher decouples the collector's log-event path from
// NATS availability. Publish never blocks on NATS: events are
// enqueued into an in-memory ring (for bursts) and, once the ring
// crosses the high-water mark or the disk spill already has pending
// entries, into an append-only JSONL file. A background goroutine
// drains spill-first then ring, retrying the underlying Publisher on
// failure.
//
// Only spill-file rotation (entries evicted to stay under the file
// cap) bumps the dropped counter. Ring-to-spill overflow is a
// transport decision, not a loss.
type BufferedPublisher struct {
	inner *Publisher
	ch    chan domain.FactEvent
	cap   int
	spill *SpillQueue

	dropped atomic.Uint64

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
	wake     chan struct{}
}

// NewBufferedPublisher wraps the given Publisher in a buffered
// asynchronous queue. capacity <= 0 falls back to BufferedCapacity.
// spill is optional; when non-nil, events that can't fit the ring at
// the high-water mark spill to disk and drain before new ring events.
func NewBufferedPublisher(inner *Publisher, capacity int, spill *SpillQueue) *BufferedPublisher {
	if capacity <= 0 {
		capacity = BufferedCapacity
	}
	return &BufferedPublisher{
		inner:  inner,
		ch:     make(chan domain.FactEvent, capacity),
		cap:    capacity,
		spill:  spill,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
		wake:   make(chan struct{}, 1),
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

// Publish enqueues the event and returns immediately. When a spill
// queue is configured, events spill to disk once the ring crosses
// the high-water mark or once the spill already has pending entries
// (so drain order remains spill → ring → live). Without a spill
// queue, the old drop-oldest ring behavior applies.
func (b *BufferedPublisher) Publish(e domain.FactEvent) error {
	if b.spill != nil {
		if !b.spill.IsEmpty() || len(b.ch) >= b.highWaterMark() {
			if err := b.spill.Append(e); err != nil {
				return err
			}
			b.notifyWake()
			return nil
		}
	}
	select {
	case b.ch <- e:
		return nil
	default:
	}
	if b.spill != nil {
		// Ring is saturated; spill to disk so the event survives.
		if err := b.spill.Append(e); err != nil {
			return err
		}
		b.notifyWake()
		return nil
	}
	// No spill configured — fall back to drop-oldest legacy behavior.
	b.dropped.Add(1)
	select {
	case <-b.ch:
	default:
	}
	for {
		select {
		case b.ch <- e:
			return nil
		case <-b.ch:
			b.dropped.Add(1)
		case <-b.stopCh:
			return nil
		}
	}
}

// Dropped returns the running count of events discarded. Covers
// in-memory-ring drops (no-spill mode) plus spill-file rotations
// (when the 100 MB cap was reached and oldest entries were evicted).
// Ring-to-spill transfers are not counted — nothing was lost.
func (b *BufferedPublisher) Dropped() uint64 {
	if b.spill == nil {
		return b.dropped.Load()
	}
	return b.dropped.Load() + b.spill.Dropped()
}

func (b *BufferedPublisher) highWaterMark() int {
	hwm := int(float64(b.cap) * BufferedHWMFraction)
	if hwm < 1 {
		hwm = 1
	}
	return hwm
}

func (b *BufferedPublisher) notifyWake() {
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

func (b *BufferedPublisher) run(ctx context.Context) {
	defer close(b.doneCh)
	for {
		// Spill first: if the disk queue has entries, drain them
		// before reading fresh ring events. This keeps the intended
		// "spill → ring → live" order so the hub's consumed_seq sees
		// a monotonic publish sequence.
		if b.spill != nil && !b.spill.IsEmpty() {
			ev, ok, err := b.spill.Peek()
			if err != nil {
				log.Printf("natsbus.BufferedPublisher: spill peek: %v", err)
				// Fall through to ring to avoid hot-looping on a
				// broken spill file; operator intervention needed.
			} else if ok {
				b.publishWithRetry(ctx, ev)
				if err := b.spill.Advance(); err != nil {
					log.Printf("natsbus.BufferedPublisher: spill advance: %v", err)
				}
				continue
			}
		}
		select {
		case <-b.stopCh:
			b.drainOnce(ctx)
			return
		case <-ctx.Done():
			b.drainOnce(ctx)
			return
		case e := <-b.ch:
			b.publishWithRetry(ctx, e)
		case <-b.wake:
			// Publish unblocked to announce a spill append; loop so
			// the next iteration services the spill queue.
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
