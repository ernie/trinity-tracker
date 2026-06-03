package collector

import (
	"strings"
	"sync"
	"testing"

	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/crypto"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
)

func TestFormatRconsetCommand(t *testing.T) {
	got := formatRconsetCommand(3, "deadbeef")
	want := "sv_cmd trinity_rconset 3 deadbeef"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRconPasswordForServerID(t *testing.T) {
	m := &ServerManager{
		mu: sync.RWMutex{},
		servers: map[int64]*serverState{
			7: {server: domain.Server{Address: "10.0.0.1:27960"}},
		},
		cfg: &config.Config{
			Q3Servers: []config.Q3Server{
				{Address: "10.0.0.1:27960", RconPassword: "hunter2"},
				{Address: "10.0.0.2:27960", RconPassword: "other-pw"},
			},
		},
	}

	pwd, ok := m.rconPasswordForServerID(7)
	if !ok || pwd != "hunter2" {
		t.Errorf("got (%q, %v); want (\"hunter2\", true)", pwd, ok)
	}

	if _, ok := m.rconPasswordForServerID(999); ok {
		t.Error("unknown serverID should not resolve")
	}
}

func TestSendRconAutoset_NoPasswordSkips(t *testing.T) {
	m := &ServerManager{
		mu: sync.RWMutex{},
		servers: map[int64]*serverState{
			1: {server: domain.Server{Address: "10.0.0.1:27960"}},
		},
		cfg: &config.Config{
			Q3Servers: []config.Q3Server{
				{Address: "10.0.0.1:27960", RconPassword: ""}, // unset
			},
		},
	}
	// Should not panic and should not attempt rcon dispatch.
	stuff := &hub.RconStuff{Role: string(hub.RconRoleOwner)}
	m.sendRconAutoset(1, 0, stuff)
}

func TestSendRconAutoset_OversizePasswordSkips(t *testing.T) {
	big := strings.Repeat("A", crypto.RconsetMaxPlaintext+1)
	m := &ServerManager{
		mu: sync.RWMutex{},
		servers: map[int64]*serverState{
			1: {server: domain.Server{Address: "10.0.0.1:27960"}},
		},
		cfg: &config.Config{
			Q3Servers: []config.Q3Server{
				{Address: "10.0.0.1:27960", RconPassword: big},
			},
		},
	}
	stuff := &hub.RconStuff{Role: string(hub.RconRoleOwner)}
	// Should hit the oversize guard and not dispatch.
	m.sendRconAutoset(1, 0, stuff)
}

func TestSendRconAutoset_UnknownServerSkips(t *testing.T) {
	m := &ServerManager{
		mu:      sync.RWMutex{},
		servers: map[int64]*serverState{},
		cfg:     &config.Config{},
	}
	stuff := &hub.RconStuff{Role: string(hub.RconRoleOwner)}
	m.sendRconAutoset(999, 0, stuff)
}
