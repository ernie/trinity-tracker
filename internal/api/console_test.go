package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/natsbus"
)

// seedConsoleWorld builds the standard fixture: a local source with one
// server, and a remote source (owned by ownerID) with one delegated and
// one non-delegated server.
func seedConsoleWorld(t *testing.T, tr *testRouter, ownerID int64) {
	t.Helper()
	ctx := context.Background()

	if err := tr.store.UpsertLocalSource(ctx, "hub-q3"); err != nil {
		t.Fatalf("UpsertLocalSource: %v", err)
	}
	if err := tr.store.CreateSource(ctx, "alice-q3", true, &ownerID); err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	tr.r.SetLocalSource("hub-q3")

	seed := func(source, key string, delegated bool) {
		srv := &domain.Server{Key: key, Address: "127.0.0.1:27960"}
		if err := tr.store.UpsertServer(ctx, source, srv); err != nil {
			t.Fatalf("UpsertServer(%s/%s): %v", source, key, err)
		}
		if delegated {
			if err := tr.store.SetServerAdminDelegation(ctx, srv.ID, true); err != nil {
				t.Fatalf("SetServerAdminDelegation: %v", err)
			}
		}
	}
	seed("hub-q3", "ffa", false)
	seed("alice-q3", "ctf", true)
	seed("alice-q3", "duel", false)
}

func consoleServers(t *testing.T, tr *testRouter, token string) []ConsoleServer {
	t.Helper()
	w := tr.do("GET", "/api/console/servers", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("console/servers: status %d body %s", w.Code, w.Body.String())
	}
	var out []ConsoleServer
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestConsoleServersRoleFiltering(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "hubadmin", true)
	ownerTok, ownerID := tr.loginAs(t, "alice", false)
	nobodyTok, _ := tr.loginAs(t, "nobody", false)
	seedConsoleWorld(t, tr, ownerID)

	// Hub admin: local ffa as owner, delegated remote ctf as hub_admin;
	// non-delegated duel omitted.
	got := map[string]string{}
	for _, s := range consoleServers(t, tr, adminTok) {
		got[s.Source+"/"+s.Key] = s.Role
	}
	want := map[string]string{"hub-q3/ffa": "owner", "alice-q3/ctf": "hub_admin"}
	if len(got) != len(want) || got["hub-q3/ffa"] != want["hub-q3/ffa"] || got["alice-q3/ctf"] != want["alice-q3/ctf"] {
		t.Errorf("admin list: got %v, want %v", got, want)
	}

	// Source owner: both of their servers (delegation irrelevant), no
	// hub servers.
	got = map[string]string{}
	for _, s := range consoleServers(t, tr, ownerTok) {
		got[s.Source+"/"+s.Key] = s.Role
	}
	if len(got) != 2 || got["alice-q3/ctf"] != "owner" || got["alice-q3/duel"] != "owner" {
		t.Errorf("owner list: got %v", got)
	}

	// Unprivileged user: empty list, not an error.
	if list := consoleServers(t, tr, nobodyTok); len(list) != 0 {
		t.Errorf("nobody list: got %v, want empty", list)
	}

	// Anonymous: 401.
	if w := tr.do("GET", "/api/console/servers", "", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous list: want 401, got %d", w.Code)
	}
}

func TestConsoleRcon(t *testing.T) {
	tr := newTestRouter(t)
	adminTok, _ := tr.loginAs(t, "hubadmin", true)
	nobodyTok, _ := tr.loginAs(t, "nobody", false)
	_, ownerID := tr.loginAs(t, "alice", false)
	seedConsoleWorld(t, tr, ownerID)

	var gotServer, gotCommand, gotRole string
	tr.r.testRconDispatch = func(server *domain.Server, command string, role natsbus.RconRole) (string, error) {
		gotServer = server.Source + "/" + server.Key
		gotCommand = command
		gotRole = string(role)
		return "map: q3dm17", nil
	}

	w := tr.do("POST", "/api/console/rcon",
		`{"source":"hub-q3","key":"ffa","command":"status"}`, adminTok)
	if w.Code != http.StatusOK {
		t.Fatalf("rcon: status %d body %s", w.Code, w.Body.String())
	}
	var resp RconResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Output != "map: q3dm17" {
		t.Errorf("output: got %q", resp.Output)
	}
	if gotServer != "hub-q3/ffa" || gotCommand != "status" || gotRole != "owner" {
		t.Errorf("dispatch saw (%s, %s, %s)", gotServer, gotCommand, gotRole)
	}

	// Audit row written with the acting user's name in the detail chain.
	audits, err := tr.store.ListSourceAudit(context.Background(), "hub-q3", 10)
	if err != nil {
		t.Fatalf("ListSourceAudit: %v", err)
	}
	if len(audits) != 1 || audits[0].Action != "rcon.exec" {
		t.Errorf("audit rows: %+v", audits)
	}

	// Unauthorized caller on a non-delegated remote server: 403, no dispatch.
	gotServer = ""
	w = tr.do("POST", "/api/console/rcon",
		`{"source":"alice-q3","key":"duel","command":"status"}`, nobodyTok)
	if w.Code != http.StatusForbidden {
		t.Errorf("forbidden rcon: want 403, got %d", w.Code)
	}
	if gotServer != "" {
		t.Error("dispatch ran despite 403")
	}

	// Unknown (source, key): 404.
	w = tr.do("POST", "/api/console/rcon",
		`{"source":"hub-q3","key":"nope","command":"status"}`, adminTok)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown server: want 404, got %d", w.Code)
	}

	// Anonymous: 401.
	w = tr.do("POST", "/api/console/rcon",
		`{"source":"hub-q3","key":"ffa","command":"status"}`, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous rcon: want 401, got %d", w.Code)
	}
}
