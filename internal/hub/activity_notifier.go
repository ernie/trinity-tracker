package hub

// ActivityNotifier is the surface the hub writer uses to report
// human-player join/leave transitions for server-activity Discord
// notifications. The concrete implementation lives in
// internal/notify; this interface keeps hub free of that import.
type ActivityNotifier interface {
	OnHumanJoin(serverID int64)
	OnHumanLeave(serverID int64)
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
