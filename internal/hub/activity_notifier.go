package hub

// ActivityNotifier receives the per-server lifecycle events behind the
// Discord server-activity notifications. internal/notify implements it,
// so hub doesn't import notify.
//
// OnMatchStart fires after the hub accepts a match_start and resets
// presence counters, so a live embed can refresh for the new match.
type ActivityNotifier interface {
	OnHumanJoin(serverID int64)
	OnHumanLeave(serverID int64)
	OnMatchStart(serverID int64)
}

// SetActivityNotifier installs n as the receiver of activity events.
// It's safe to call before or after Start; nil clears it.
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
