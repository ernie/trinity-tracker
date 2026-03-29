package collector

import (
	"context"
	"fmt"
	"log"
	"time"
)

// pendingGreeting stores the info needed to greet a player after handshake completes.
type pendingGreeting struct {
	serverID        int64
	clientID        int
	playerID        int64
	playerName      string
	cleanName       string
	isVR            bool
	isTrinityEngine bool
	guidLinked      bool
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
// If the handshake arrives before the timeout, the greeting is sent immediately.
// If the timeout fires, a warning is sent instead (the QVM kicks after 10s).
func (m *ServerManager) scheduleGreetingAfterHandshake(ctx context.Context, state *serverState, serverID int64, clientID int, playerID int64, playerName, cleanName string, isVR, isTrinityEngine bool) {
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
		playerID:        playerID,
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
		m.sendCenterPrint(serverID, clientID, "^1Trinity client required\n^7You will be disconnected.\nVisit ^5trinity.ernie.io/getting-started")
		m.sendPrint(serverID, clientID, "^1This server requires a Trinity client. Visit ^5trinity.ernie.io/getting-started ^1to download.")
	})

	state.pendingGreetings[clientID] = pg
}

// completeHandshakeGreeting is called when a TrinityHandshake event is received.
// It cancels the timeout and sends the normal greeting.
func (m *ServerManager) completeHandshakeGreeting(ctx context.Context, state *serverState, clientID int) {
	if state.pendingGreetings == nil {
		return
	}
	pg, ok := state.pendingGreetings[clientID]
	if !ok {
		return
	}
	pg.timer.Stop()
	delete(state.pendingGreetings, clientID)

	// Send the normal greeting
	go m.greetPlayer(ctx, pg.serverID, pg.clientID, pg.playerID, pg.playerName, pg.cleanName, pg.isVR, pg.isTrinityEngine, pg.guidLinked)
}

func (m *ServerManager) handleTrinityHandshake(ctx context.Context, serverID int64, state *serverState, data TrinityHandshakeData) {
	nonce, nonceOk := state.trinityNonces[data.ClientNum]
	delete(state.trinityNonces, data.ClientNum)

	// Store client engine/version on the session
	if client, clientOk := state.clients[data.ClientNum]; clientOk && client.sessionID > 0 {
		if err := m.store.UpdateSessionClientInfo(ctx, client.sessionID, data.Engine, data.Version); err != nil {
			log.Printf("Error updating session client info: %v", err)
		}
	}

	// Validate auth token if provided
	if data.Username != "" && data.TokenHash != "" {
		if !nonceOk {
			log.Printf("Trinity auth: no nonce for client %d on server %d", data.ClientNum, serverID)
			m.sendTrinityAuthFail(serverID, data.ClientNum)
		} else {
			playerID, token, err := m.store.GetGameTokenByUsername(ctx, data.Username)
			if err != nil {
				log.Printf("Trinity auth: no game token for user %s: %v", data.Username, err)
				m.sendTrinityAuthFail(serverID, data.ClientNum)
			} else {
				expected := sipHashHex(token, nonce)
				if expected != data.TokenHash {
					log.Printf("Trinity auth: hash mismatch for user %s: expected %s got %s", data.Username, expected, data.TokenHash)
					m.sendTrinityAuthFail(serverID, data.ClientNum)
				} else {
					log.Printf("Trinity auth verified: client %d user %s on server %d", data.ClientNum, data.Username, serverID)
					// Auto-associate the GUID with the user's player
					if client, ok := state.clients[data.ClientNum]; ok && client.guid != "" {
						merged, err := m.store.AssociateGUIDWithPlayer(ctx, client.guid, playerID)
						if err != nil {
							log.Printf("Trinity auth: failed to associate GUID %s with player %d: %v", client.guid, playerID, err)
						} else if merged {
							log.Printf("Trinity auth: linked GUID %s to player %d", client.guid, playerID)
							if pg, ok := state.pendingGreetings[data.ClientNum]; ok {
								pg.guidLinked = true
							}
						}
					}
				}
			}
		}
	}

	// Complete the pending greeting (always — greeting is GUID-based, not auth-dependent)
	m.completeHandshakeGreeting(ctx, state, data.ClientNum)
}

func (m *ServerManager) sendTrinityAuthFail(serverID int64, clientNum int) {
	go func() {
		cmd := fmt.Sprintf("trinity_auth_fail %d", clientNum)
		if _, err := m.ExecuteRcon(serverID, cmd); err != nil {
			log.Printf("Failed to send trinity_auth_fail to server %d client %d: %v", serverID, clientNum, err)
		}
	}()
}

// sipHashHex matches the BG_HashKeyed implementation in the QVM:
// SipHash-2-4 with 128-bit output, keyed by token, message is nonce.
func sipHashHex(key, message string) string {
	k0, k1 := deriveKey(key)
	msg := []byte(message)

	v0 := k0 ^ 0x736f6d6570736575
	v1 := k1 ^ 0x646f72616e646f6d
	v2 := k0 ^ 0x6c7967656e657261
	v3 := k1 ^ 0x7465646279746573

	// 128-bit output tag
	v1 ^= 0xee

	blocks := len(msg) / 8
	for i := 0; i < blocks; i++ {
		m := uint64(msg[i*8]) |
			uint64(msg[i*8+1])<<8 |
			uint64(msg[i*8+2])<<16 |
			uint64(msg[i*8+3])<<24 |
			uint64(msg[i*8+4])<<32 |
			uint64(msg[i*8+5])<<40 |
			uint64(msg[i*8+6])<<48 |
			uint64(msg[i*8+7])<<56
		v3 ^= m
		v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
		v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
		v0 ^= m
	}

	// Last block with length byte
	var m uint64
	left := len(msg) & 7
	for j := left - 1; j >= 0; j-- {
		m <<= 8
		m |= uint64(msg[blocks*8+j])
	}
	m |= uint64(len(msg)&0xff) << 56
	v3 ^= m
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0 ^= m

	// First finalization
	v2 ^= 0xee
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	hash0 := v0 ^ v1 ^ v2 ^ v3

	// Second finalization
	v1 ^= 0xdd
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	hash1 := v0 ^ v1 ^ v2 ^ v3

	return fmt.Sprintf("%08x%08x%08x%08x",
		uint32(hash0), uint32(hash0>>32),
		uint32(hash1), uint32(hash1>>32))
}

func sipRound(v0, v1, v2, v3 uint64) (uint64, uint64, uint64, uint64) {
	v0 += v1
	v2 += v3
	v1 = v1<<13 | v1>>(64-13)
	v3 = v3<<16 | v3>>(64-16)
	v1 ^= v0
	v3 ^= v2
	v0 = v0<<32 | v0>>(64-32)
	v2 += v1
	v0 += v3
	v1 = v1<<17 | v1>>(64-17)
	v3 = v3<<21 | v3>>(64-21)
	v1 ^= v2
	v3 ^= v0
	v2 = v2<<32 | v2>>(64-32)
	return v0, v1, v2, v3
}

// deriveKey folds a variable-length key into two uint64 SipHash key halves.
// Must match the DeriveKey function in bg_hash.c exactly.
func deriveKey(key string) (uint64, uint64) {
	h := [4]uint32{0x736f6d65, 0x646f7261, 0x6c796765, 0x74656462}
	for i := 0; i < len(key); i++ {
		h[i&3] ^= uint32(key[i])
		h[i&3] *= 0x01000193
	}
	k0 := uint64(h[0])<<32 | uint64(h[1])
	k1 := uint64(h[2])<<32 | uint64(h[3])
	return k0, k1
}
