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

// Broadcast categories. A client picks its categories by sending
// {"type":"subscribe","categories":["server","activity"]}.
const (
	categoryServer   = "server"
	categoryActivity = "activity"
)

// categoryOf routes an event to "server" (live cards) or "activity"
// (the drawer feed). Obelisk damage and skull pickups light indicators
// on the cards, so they ride "server" with the drawer closed too.
func categoryOf(eventType string) string {
	switch eventType {
	case domain.EventServerUpdate,
		domain.EventObeliskDamage,
		domain.EventSkullPickup:
		return categoryServer
	}
	return categoryActivity
}

// getClientIP trusts X-Real-IP, which nginx sets from $remote_addr.
// X-Forwarded-For passes through nginx untouched, so a client can
// forge it.
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
		return true
	},
}

type broadcastMsg struct {
	category string
	bytes    []byte
}

// WebSocketClient is one connected browser.
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

// subscribeMsg is the only message the server reads from a client.
type subscribeMsg struct {
	Type       string   `json:"type"`
	Categories []string `json:"categories"`
}

// WebSocketHub fans events out to connected clients.
type WebSocketHub struct {
	clients    map[*WebSocketClient]bool
	broadcast  chan broadcastMsg
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	mu         sync.RWMutex
}

// NewWebSocketHub returns a hub; start it with Run.
func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		clients:    make(map[*WebSocketClient]bool),
		broadcast:  make(chan broadcastMsg, 256),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
	}
}

// Run is the hub's main loop.
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
					// A client that can't keep up is dropped.
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast queues an event for every client subscribed to its
// category, dropping it when the queue is full.
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

// ClientCount returns the number of connected clients.
func (h *WebSocketHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

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
		// Both categories until the client subscribes.
		subs: map[string]bool{categoryServer: true, categoryActivity: true},
	}

	r.wsHub.register <- client

	go client.writePump()
	go client.readPump()
}

// readPump applies subscribe messages and ignores anything else.
func (c *WebSocketClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

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
			continue
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

			// Queued messages share the frame, one per line.
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
