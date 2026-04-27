package hub

import "github.com/ernie/trinity-tracker/internal/domain"

// LiveEventPublisher tees live events onto the distributed bus.
type LiveEventPublisher interface {
	PublishLive(e domain.Event) error
}

// LiveEventSink receives enriched live events for WebSocket broadcast.
type LiveEventSink interface {
	Broadcast(e domain.Event)
}
