package directory

import "sync/atomic"

// metrics counts directory events; Server.Stats exposes them so tests
// can check counts.
type metrics struct {
	heartbeatsRecv     atomic.Uint64
	heartbeatsRejected atomic.Uint64
	probesSent         atomic.Uint64
	infoResponsesRecv  atomic.Uint64
	validations        atomic.Uint64
	getserversRecv     atomic.Uint64
	getserversReplied  atomic.Uint64
	rateLimited        atomic.Uint64
	parseErrors        atomic.Uint64
}

// Stats is a point-in-time snapshot of the counters.
type Stats struct {
	HeartbeatsReceived    uint64
	HeartbeatsRejected    uint64
	ProbesSent            uint64
	InfoResponsesReceived uint64
	Validations           uint64
	GetserversReceived    uint64
	GetserversReplied     uint64
	RateLimited           uint64
	ParseErrors           uint64
	RegistrySize          int
	GateSize              int
}
