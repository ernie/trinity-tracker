package natsbus

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

const (
	BufferedCapacity     = 10_000 // ~5 MB resident at ~500B/envelope
	BufferedHWMFraction  = 0.8    // ring fill ratio at which spill engages
	BufferedRetryBackoff = 250 * time.Millisecond
)

// BufferedPublisher decouples the publish call site from NATS
// availability. Publish never blocks: events go into an in-memory ring
// and, past the high-water mark (or if spill already has entries), into
// an on-disk JSONL queue. A background goroutine drains spill → ring
// with retry. Only spill-file rotation counts as a drop; ring-to-spill
// overflow is a transport decision, not a loss.
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

// NewBufferedPublisher wraps inner in an async queue. capacity <= 0
// falls back to BufferedCapacity. spill is optional.
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

func (b *BufferedPublisher) Start(ctx context.Context) {
	go b.run(ctx)
}

// Stop drains once with a short deadline and tears down.
func (b *BufferedPublisher) Stop() {
	b.stopOnce.Do(func() { close(b.stopCh) })
	<-b.doneCh
}

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
		if err := b.spill.Append(e); err != nil {
			return err
		}
		b.notifyWake()
		return nil
	}
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

// Dropped counts ring drop-oldest events plus spill-file rotations.
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
		// Drain spill ahead of ring so consumed_seq stays monotonic.
		if b.spill != nil && !b.spill.IsEmpty() {
			ev, ok, err := b.spill.Peek()
			if err != nil {
				log.Printf("natsbus.BufferedPublisher: spill peek: %v", err)
				// Fall through to ring so a broken spill file doesn't hot-loop.
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
		}
	}
}

// publishWithRetry retries inner.Publish until success, stop, or ctx cancel.
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

// drainOnce makes a bounded final pass over the ring on shutdown.
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
