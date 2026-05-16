package hub

// ActivityNotifier is the surface the hub writer uses to report
// per-server lifecycle events for the server-activity Discord
// notifications. The concrete implementation lives in
// internal/notify; this interface keeps hub free of that import.
//
// OnMatchStart fires after the hub has accepted a match_start
// fact event and reset presence counters. The notifier uses it
// to refresh the live embed when a server is mid-session and a
// new map (or new match on the same map) starts.
type ActivityNotifier interface {
	OnHumanJoin(serverID int64)
	OnHumanLeave(serverID int64)
	OnMatchStart(serverID int64)
}

// SetActivityNotifier installs n as the receiver of human join/leave
// callbacks. Safe to call before or after Start; on every event the
// handler loads the latest value atomically. Passing nil clears any
// previously installed notifier.
func (w *Writer) SetActivityNotifier(n ActivityNotifier) {
	if n == nil {
		w.activityNotifier.Store((*ActivityNotifier)(nil))
		return
	}
	w.activityNotifier.Store(&n)
}

func (w *Writer) loadActivityNotifier() ActivityNotifier {
	p := w.activityNotifier.Load()
	if p == nil {
		return nil
	}
	return *p
}
