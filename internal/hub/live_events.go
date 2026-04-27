package hub

import "github.com/ernie/trinity-tracker/internal/domain"

// LiveEventPublisher is the call-site contract for teeing live events
// (frags, awards, chat, flag actions, etc.) onto the distributed bus.
// Satisfied by natsbus.LivePublisher in collector-only mode; unused
// (nil) in standalone and hub+collector modes where the local
// manager.Events() channel is the only transport.
type LiveEventPublisher interface {
	PublishLive(e domain.Event) error
}

// LiveEventSink receives decoded + enriched live events on the hub
// side so it can push them to WebSocket clients. Implemented by the
// API router's wsHub wrapper.
type LiveEventSink interface {
	Broadcast(e domain.Event)
}
