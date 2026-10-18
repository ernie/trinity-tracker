package collector

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
)

// pendingGreeting holds the client info needed to greet a player after
// the Trinity handshake arrives (or the timeout fires).
type pendingGreeting struct {
	serverID        int64
	clientID        int
	guid            string
	playerName      string
	cleanName       string
	isVR            bool
	isTrinityEngine bool
	timer           *time.Timer
}

// handshakeRequired reports whether the server's most recent InitGame
// line in the log enabled g_trinityHandshake.
func (m *ServerManager) handshakeRequired(state *serverState) bool {
	return state.handshakeRequired
}

// scheduleGreetingAfterHandshake holds the greeting until the handshake
// arrives. If the timeout fires first, the player gets a warning instead,
// since the QVM kicks after 10 seconds.
func (m *ServerManager) scheduleGreetingAfterHandshake(ctx context.Context, state *serverState, serverID int64, clientID int, guid, playerName, cleanName string, isVR, isTrinityEngine bool) {
	if state.pendingGreetings == nil {
		state.pendingGreetings = make(map[int]*pendingGreeting)
	}

	if pg, ok := state.pendingGreetings[clientID]; ok {
		pg.timer.Stop()
		delete(state.pendingGreetings, clientID)
	}

	pg := &pendingGreeting{
		serverID:        serverID,
		clientID:        clientID,
		guid:            guid,
		playerName:      playerName,
		cleanName:       cleanName,
		isVR:            isVR,
		isTrinityEngine: isTrinityEngine,
	}

	pg.timer = time.AfterFunc(3*time.Second, func() {
		m.mu.Lock()
		if _, ok := state.pendingGreetings[clientID]; ok {
			delete(state.pendingGreetings, clientID)
		}
		m.mu.Unlock()

		log.Printf("No Trinity handshake from client %d on server %d; sending a warning", clientID, serverID)
		m.sendCenterPrint(serverID, clientID, "^1Trinity client required\n^7You will be disconnected.\nVisit ^5trinity.run/docs")
		m.sendPrint(serverID, clientID, "^1This server needs the Trinity client. ^7Get it at ^5trinity.run/docs^7.")
	})

	state.pendingGreetings[clientID] = pg
}

func (m *ServerManager) handleTrinityHandshake(ctx context.Context, serverID int64, state *serverState, data TrinityHandshakeData) {
	nonce, nonceOk := state.trinityNonces[data.ClientNum]
	delete(state.trinityNonces, data.ClientNum)

	client, clientOk := state.clients[data.ClientNum]

	if clientOk && client.guid != "" && !client.isBot {
		m.pub.Publish(domain.FactEvent{
			Type:      domain.FactTrinityHandshake,
			ServerID:  serverID,
			Timestamp: time.Now().UTC(),
			Data: domain.TrinityHandshakeData{
				GUID:          client.guid,
				ClientEngine:  data.Engine,
				ClientVersion: data.Version,
				Platform:      data.Platform,
			},
		})
	}

	// On servers without g_trinityHandshake there's no pending greeting:
	// ClientBegin already greeted the player.
	var pg *pendingGreeting
	if state.pendingGreetings != nil {
		pg = state.pendingGreetings[data.ClientNum]
		if pg != nil {
			pg.timer.Stop()
			delete(state.pendingGreetings, data.ClientNum)
		}
	}

	if pg == nil {
		return
	}

	// The hub greets a nil proof as unauthenticated.
	var auth *hub.AuthProof
	if data.Username != "" && data.TokenHash != "" && nonceOk {
		auth = &hub.AuthProof{
			Username:  data.Username,
			Nonce:     nonce,
			TokenHash: data.TokenHash,
		}
	} else if data.Username != "" && data.TokenHash != "" && !nonceOk {
		log.Printf("Trinity auth: no nonce for client %d on server %d", data.ClientNum, serverID)
	}

	go m.performGreet(ctx, pg.serverID, pg.clientID, pg.guid, pg.playerName, pg.cleanName, pg.isVR, pg.isTrinityEngine, auth)
}

func (m *ServerManager) sendTrinityAuthFail(serverID int64, clientNum int) {
	go func() {
		cmd := fmt.Sprintf("trinity_auth_fail %d", clientNum)
		if _, err := m.ExecuteRcon(serverID, cmd); err != nil {
			log.Printf("Failed to send trinity_auth_fail to server %d client %d: %v", serverID, clientNum, err)
		}
	}()
}

// sendTrinityAuthOk tells the QVM the hub verified this client's
// identity. userType is 1 for a verified user, 2 for an admin. A
// non-empty canonicalName goes as a quoted third argument so the QVM
// locks the in-game name to the account's display name.
func (m *ServerManager) sendTrinityAuthOk(serverID int64, clientNum, userType int, canonicalName string) {
	go func() {
		// The game's tokenizer has no escape for a quote inside a quoted
		// argument; spaces and color codes pass through.
		safeName := strings.ReplaceAll(canonicalName, "\"", "")
		var cmd string
		if safeName == "" {
			cmd = fmt.Sprintf("trinity_auth_ok %d %d", clientNum, userType)
		} else {
			cmd = fmt.Sprintf("trinity_auth_ok %d %d \"%s\"", clientNum, userType, safeName)
		}
		if _, err := m.ExecuteRcon(serverID, cmd); err != nil {
			log.Printf("Failed to send trinity_auth_ok to server %d client %d: %v", serverID, clientNum, err)
		}
	}()
}

func (m *ServerManager) sendTrinityNameConflict(serverID int64, clientNum int) {
	go func() {
		cmd := fmt.Sprintf("trinity_name_conflict %d", clientNum)
		if _, err := m.ExecuteRcon(serverID, cmd); err != nil {
			log.Printf("Failed to send trinity_name_conflict to server %d client %d: %v", serverID, clientNum, err)
		}
	}()
}
