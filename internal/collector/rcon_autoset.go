package collector

import (
	"fmt"
	"log"

	"github.com/ernie/trinity-tracker/internal/crypto"
	"github.com/ernie/trinity-tracker/internal/hub"
)

// sendRconAutoset pushes an encrypted rconPassword to one connected
// client whose authenticated player may rcon this server. The hub
// supplies the epoch nonce and key; the password comes from this
// collector's config.
//
// Failures are only logged: the welcome flow is already under way, and
// a transient rcon timeout shouldn't hold it up.
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

// formatRconsetCommand builds the rcon line the mod's sv_cmd helper
// forwards to the client. The blob is plain hex, so the helper's
// argument escaping (it strips " and rewrites \n) leaves it intact.
func formatRconsetCommand(clientID int, hexblob string) string {
	return fmt.Sprintf("sv_cmd trinity_rconset %d %s", clientID, hexblob)
}

// rconPasswordForServerID returns the configured rcon password for a
// running server, matched by address as ExecuteRcon does.
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
