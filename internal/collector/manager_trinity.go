package collector

import (
	"context"
	"fmt"
	"log"
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

// handshakeRequired checks if the server has g_trinityHandshake enabled.
func (m *ServerManager) handshakeRequired(state *serverState) bool {
	if state.status == nil {
		return false
	}
	return state.status.ServerVars["g_trinityhandshake"] == "1"
}

// scheduleGreetingAfterHandshake stores a pending greeting and starts a timeout.
// If the handshake arrives before the timeout, performGreet is called with
// the auth bundle. If the timeout fires, a warning is sent instead
// (the QVM kicks after 10s).
func (m *ServerManager) scheduleGreetingAfterHandshake(ctx context.Context, state *serverState, serverID int64, clientID int, guid, playerName, cleanName string, isVR, isTrinityEngine bool) {
	if state.pendingGreetings == nil {
		state.pendingGreetings = make(map[int]*pendingGreeting)
	}

	// Cancel any existing pending greeting for this client
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
		// No handshake received in time — warn the player
		m.mu.Lock()
		if _, ok := state.pendingGreetings[clientID]; ok {
			delete(state.pendingGreetings, clientID)
		}
		m.mu.Unlock()

		log.Printf("No Trinity handshake from client %d on server %d — sending warning", clientID, serverID)
		m.sendCenterPrint(serverID, clientID, "^1Trinity client required\n^7You will be disconnected.\nVisit ^5trinity.run/docs")
		m.sendPrint(serverID, clientID, "^1This server requires a Trinity client. Visit ^5trinity.run/docs ^1to download.")
	})

	state.pendingGreetings[clientID] = pg
}

func (m *ServerManager) handleTrinityHandshake(ctx context.Context, serverID int64, state *serverState, data TrinityHandshakeData) {
	nonce, nonceOk := state.trinityNonces[data.ClientNum]
	delete(state.trinityNonces, data.ClientNum)

	client, clientOk := state.clients[data.ClientNum]

	// Stamp client engine/version on the session via the hub writer.
	if clientOk && client.guid != "" && !client.isBot {
		m.pub.Publish(domain.FactEvent{
			Type:      domain.FactTrinityHandshake,
			ServerID:  serverID,
			Timestamp: time.Now().UTC(),
			Data: domain.TrinityHandshakeData{
				GUID:          client.guid,
				ClientEngine:  data.Engine,
				ClientVersion: data.Version,
			},
		})
	}

	// Pull the pending greeting (if any) and cancel its timeout. The
	// greet RPC itself runs whether or not a handshake was scheduled —
	// for servers without g_trinityhandshake the ClientBegin path
	// already fired performGreet.
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

	// Build auth bundle (nil if the client didn't supply credentials or
	// we never stashed a nonce). The hub treats nil as unauthenticated.
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

