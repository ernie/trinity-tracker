package natsbus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ernie/trinity-tracker/internal/domain"
)

func TestRegistrarKickPublishes(t *testing.T) {
	s := startTestServer(t)
	nc := connectInProcess(t, s)

	got := make(chan domain.Registration, 4)
	sub, err := nc.Subscribe(RegistrationSubjectPrefix+"src", func(m *nats.Msg) {
		var reg domain.Registration
		if json.Unmarshal(m.Data, &reg) == nil {
			got <- reg
		}
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Long interval so the ticker never fires during the test; any
	// publish we observe past the initial one came from Kick.
	r, err := NewRegistrar(nc, "src", "v", "", func() []domain.RegdServer { return nil }, time.Hour)
	if err != nil {
		t.Fatalf("NewRegistrar: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx) // immediate initial publish

	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("no initial publish")
	}

	r.Kick() // out-of-band publish despite the 1h ticker
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("Kick did not trigger a publish")
	}
}
