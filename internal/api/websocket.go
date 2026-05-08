package api

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/gorilla/websocket"
)

// Broadcast categories. The frontend opts in/out per category by sending
// {"type":"subscribe","categories":["server","activity"]}. Default for a
// fresh client is both — preserves existing behavior for any consumer
// that doesn't speak the protocol.
const (
	categoryServer   = "server"
	categoryActivity = "activity"
)

// categoryOf returns the broadcast category for an event type. "server"
// covers events that drive the live server-card display; "activity"
// covers joins/leaves/chat/awards and everything else the activity
// drawer renders.
func categoryOf(eventType string) string {
	if eventType == domain.EventServerUpdate {
		return categoryServer
	}
	return categoryActivity
}

// getClientIP extracts the real client IP. Trusts X-Real-IP because
// our nginx config sets it from $remote_addr; X-Forwarded-For is
// ignored since nginx neither strips nor rewrites it, leaving it
// client-controllable and spoofable.
func getClientIP(r *http.Request) string {
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// broadcastMsg pairs a serialized event with its routing category so the
// hub can fan out per-client based on subscriptions.
type broadcastMsg struct {
	category string
	bytes    []byte
}

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	hub        *WebSocketHub
	conn       *websocket.Conn
	send       chan []byte
	remoteAddr string

	subsMu sync.RWMutex
	subs   map[string]bool
}

func (c *WebSocketClient) subscribed(category string) bool {
	c.subsMu.RLock()
	defer c.subsMu.RUnlock()
	return c.subs[category]
}

// subscribeMsg is the only client→server message shape we recognize.
// Anything else is ignored.
type subscribeMsg struct {
	Type       string   `json:"type"`
	Categories []string `json:"categories"`
}

// WebSocketHub manages WebSocket connections
type WebSocketHub struct {
	clients    map[*WebSocketClient]bool
	broadcast  chan broadcastMsg
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	mu         sync.RWMutex
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		clients:    make(map[*WebSocketClient]bool),
		broadcast:  make(chan broadcastMsg, 256),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
	}
}

// Run starts the hub's main loop
func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("WebSocket client connected from %s (%d total)", client.remoteAddr, len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("WebSocket client disconnected from %s (%d total)", client.remoteAddr, len(h.clients))

		case msg := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				if !client.subscribed(msg.category) {
					continue
				}
				select {
				case client.send <- msg.bytes:
				default:
					// Client's buffer is full, close connection
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends an event to all subscribed clients
func (h *WebSocketHub) Broadcast(event domain.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Error marshaling event: %v", err)
		return
	}

	msg := broadcastMsg{category: categoryOf(event.Type), bytes: data}
	select {
	case h.broadcast <- msg:
	default:
		log.Printf("Broadcast channel full, dropping event")
	}
}

// ClientCount returns the number of connected clients
func (h *WebSocketHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// handleWebSocket upgrades HTTP to WebSocket and manages the connection
func (r *Router) handleWebSocket(w http.ResponseWriter, req *http.Request) {
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &WebSocketClient{
		hub:        r.wsHub,
		conn:       conn,
		send:       make(chan []byte, 256),
		remoteAddr: getClientIP(req),
		// Default both categories on so consumers that don't speak the
		// subscribe protocol behave exactly as they did before.
		subs: map[string]bool{categoryServer: true, categoryActivity: true},
	}

	r.wsHub.register <- client

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

// readPump reads messages from the WebSocket. Recognizes only the
// subscribe shape; ignores anything else.
func (c *WebSocketClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	// 2KB is generous for a subscribe payload (categories list); the
	// previous 512-byte limit was too tight for protocol headroom.
	c.conn.SetReadLimit(2048)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		var sub subscribeMsg
		if err := json.Unmarshal(data, &sub); err != nil {
			continue // unknown shape — ignore
		}
		if sub.Type != "subscribe" {
			continue
		}
		next := make(map[string]bool, len(sub.Categories))
		for _, cat := range sub.Categories {
			next[cat] = true
		}
		c.subsMu.Lock()
		c.subs = next
		c.subsMu.Unlock()
	}
}

// writePump sends messages to the WebSocket
func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Drain queued messages into this write
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
