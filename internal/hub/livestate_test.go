package hub

import "testing"

func TestLiveState(t *testing.T) {
	var nilLS *LiveState
	if nilLS.Get("a", "b") {
		t.Fatal("nil LiveState.Get should be false")
	}

	ls := NewLiveState()
	if ls.Get("hub", "ffa") {
		t.Fatal("empty LiveState reports live")
	}
	ls.Set("hub", "ffa", true)
	if !ls.Get("hub", "ffa") {
		t.Fatal("Set(true) not reflected")
	}
	if ls.Get("hub", "1v1") || ls.Get("other", "ffa") {
		t.Fatal("key collision across source/key")
	}
	ls.Set("hub", "ffa", false)
	if ls.Get("hub", "ffa") {
		t.Fatal("Set(false) did not clear")
	}
}

func TestLiveStateDelay(t *testing.T) {
	var nilLS *LiveState
	if nilLS.GetDelay("a", "b") != 0 {
		t.Fatal("nil LiveState.GetDelay should be 0")
	}

	ls := NewLiveState()
	if ls.GetDelay("hub", "ffa") != 0 {
		t.Fatal("unset delay should be 0")
	}
	ls.SetDelay("hub", "ffa", 5)
	if got := ls.GetDelay("hub", "ffa"); got != 5 {
		t.Fatalf("GetDelay = %d, want 5", got)
	}
	// Delay is per (source,key) and independent of liveness.
	if ls.GetDelay("hub", "1v1") != 0 || ls.GetDelay("other", "ffa") != 0 {
		t.Fatal("delay key collision across source/key")
	}
	ls.Set("hub", "ffa", false) // clearing liveness must not drop the delay
	if ls.GetDelay("hub", "ffa") != 5 {
		t.Fatal("delay should survive a liveness clear")
	}
}

func TestLiveStateViewers(t *testing.T) {
	var nilLS *LiveState
	if nilLS.GetViewers("a", "b") != 0 {
		t.Fatal("nil LiveState.GetViewers should be 0")
	}

	ls := NewLiveState()
	if ls.GetViewers("hub", "ffa") != 0 {
		t.Fatal("unset viewers should be 0")
	}
	ls.SetViewers("hub", "ffa", 3)
	if got := ls.GetViewers("hub", "ffa"); got != 3 {
		t.Fatalf("GetViewers = %d, want 3", got)
	}
	if ls.GetViewers("hub", "1v1") != 0 || ls.GetViewers("other", "ffa") != 0 {
		t.Fatal("viewers key collision across source/key")
	}
	// Independent of liveness: gap-parked viewers keep counting while not live.
	ls.Set("hub", "ffa", false)
	if ls.GetViewers("hub", "ffa") != 3 {
		t.Fatal("viewers should survive a liveness clear")
	}
	ls.SetViewers("hub", "ffa", 0) // next heartbeat reports the drop
	if ls.GetViewers("hub", "ffa") != 0 {
		t.Fatal("SetViewers(0) did not clear")
	}
}
