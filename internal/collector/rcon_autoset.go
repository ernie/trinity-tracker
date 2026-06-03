package collector

import (
	"fmt"
	"log"

	"github.com/ernie/trinity-tracker/internal/crypto"
	"github.com/ernie/trinity-tracker/internal/hub"
)

// sendRconAutoset stuffs an encrypted rconPassword down to a single
// connected client. Called from performGreet when the hub reports the
// authenticated player is rcon-authorized for this server. The hub
// supplies (EpochNonce, Key); the collector supplies the rcon password
// from its own cfg, encrypts, and pushes via sv_cmd.
//
// Hard-coded to use the existing `sv_cmd <subcmd> <clientnum> <args>`
// mod helper (game/g_svcmds.c:528). The encrypted payload is a single
// hex string so it survives the mod's arg-escape rules untouched.
//
// Fire-and-forget by design. Failures are logged but never propagated
// back to the player — the welcome flow has already started by the
// time this runs and we don't want to block it on a transient rcon
// timeout.
func (m *ServerManager) sendRconAutoset(serverID int64, clientID int, stuff *hub.RconStuff) {
	pwd, ok := m.rconPasswordForServerID(serverID)
	if !ok || pwd == "" {
		log.Printf("rcon-autoset: no rcon password configured for server %d; skipping", serverID)
		return
	}
	if len(pwd) > crypto.RconsetMaxPlaintext {
		log.Printf("rcon-autoset: password for server %d is %d bytes (>%d); refusing to stuff",
			serverID, len(pwd), crypto.RconsetMaxPlaintext)
		return
	}

	hexblob, err := crypto.EncryptRconset(stuff.Key, stuff.EpochNonce, []byte(pwd))
	if err != nil {
		log.Printf("rcon-autoset: encrypt failed for server %d client %d: %v", serverID, clientID, err)
		return
	}

	cmd := formatRconsetCommand(clientID, hexblob)
	go func() {
		if _, err := m.ExecuteRcon(serverID, cmd); err != nil {
			log.Printf("rcon-autoset: ExecuteRcon failed for server %d client %d: %v",
				serverID, clientID, err)
			return
		}
		log.Printf("rcon-autoset: stuffed encrypted rconPassword to server %d client %d (role=%s)",
			serverID, clientID, stuff.Role)
	}()
}

// formatRconsetCommand builds the rcon command line that the mod's
// sv_cmd helper (game/g_svcmds.c:528) forwards to the named client.
// The encrypted blob is a single hex string with no special chars, so
// the mod's arg-escape rules (which strip " and rewrite \n) leave it
// untouched.
func formatRconsetCommand(clientID int, hexblob string) string {
	return fmt.Sprintf("sv_cmd trinity_rconset %d %s", clientID, hexblob)
}

// rconPasswordForServerID returns the configured rcon password for a
// running server, matched by the in-memory state's address. Mirrors
// the same lookup ExecuteRcon does internally.
func (m *ServerManager) rconPasswordForServerID(serverID int64) (string, bool) {
	m.mu.RLock()
	state, ok := m.servers[serverID]
	m.mu.RUnlock()
	if !ok {
		return "", false
	}
	for _, srv := range m.cfg.Q3Servers {
		if srv.Address == state.server.Address {
			return srv.RconPassword, true
		}
	}
	return "", false
}
