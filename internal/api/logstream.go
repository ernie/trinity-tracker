package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ernie/trinity-tools/internal/collector"
	"github.com/ernie/trinity-tools/internal/storage"
	"github.com/gorilla/websocket"
)

// LogMessage is the message format for log streaming
type LogMessage struct {
	Type    string   `json:"type"`              // "initial", "lines", "error"
	Lines   []string `json:"lines,omitempty"`   // log lines
	Message string   `json:"message,omitempty"` // error message
}

// LogStreamClient represents a client subscribed to log streaming
type LogStreamClient struct {
	conn     *websocket.Conn
	send     chan []byte
	serverID int64
	manager  *LogStreamManager
}

// LogStreamManager manages log streaming to WebSocket clients
type LogStreamManager struct {
	mu      sync.RWMutex
	store   *storage.Store
	tailers map[int64]*collector.RawLogTailer     // serverID -> tailer
	clients map[int64]map[*LogStreamClient]bool   // serverID -> set of clients
}

// NewLogStreamManager creates a new log stream manager
func NewLogStreamManager(store *storage.Store) *LogStreamManager {
	return &LogStreamManager{
		store:   store,
		tailers: make(map[int64]*collector.RawLogTailer),
		clients: make(map[int64]map[*LogStreamClient]bool),
	}
}

// Subscribe adds a client to log streaming for a server
func (m *LogStreamManager) Subscribe(client *LogStreamClient, serverID int64) ([]string, error) {
	// Get server to find log path
	server, err := m.store.GetServerByID(context.Background(), serverID)
	if err != nil {
		return nil, err
	}

	if server.LogPath == "" {
		return nil, nil // No log configured
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize client set for this server if needed
	if m.clients[serverID] == nil {
		m.clients[serverID] = make(map[*LogStreamClient]bool)
	}

	// Create tailer if first subscriber for this server
	tailer := m.tailers[serverID]
	if tailer == nil {
		tailer = collector.NewRawLogTailer(server.LogPath)
		m.tailers[serverID] = tailer
	}

	// Read initial lines before starting tail
	lines, err := tailer.ReadLastNLines(500)
	if err != nil {
		log.Printf("Error reading initial log lines for server %d: %v", serverID, err)
		lines = []string{} // Continue with empty initial content
	}

	// Add client to subscribers
	m.clients[serverID][client] = true
	client.serverID = serverID

	// Start tailer if this is the first subscriber
	if len(m.clients[serverID]) == 1 {
		if err := tailer.Start(); err != nil {
			log.Printf("Error starting log tailer for server %d: %v", serverID, err)
		} else {
			// Start goroutine to forward lines to clients
			go m.forwardLines(serverID, tailer)
		}
	}

	log.Printf("Log stream client subscribed to server %d (%d total)", serverID, len(m.clients[serverID]))
	return lines, nil
}

// Unsubscribe removes a client from log streaming
func (m *LogStreamManager) Unsubscribe(client *LogStreamClient) {
	m.mu.Lock()
	defer m.mu.Unlock()

	serverID := client.serverID
	if serverID == 0 {
		return
	}

	// Remove client from subscribers
	if clients, ok := m.clients[serverID]; ok {
		delete(clients, client)
		log.Printf("Log stream client unsubscribed from server %d (%d remaining)", serverID, len(clients))

		// Stop tailer if no more subscribers
		if len(clients) == 0 {
			if tailer, ok := m.tailers[serverID]; ok {
				tailer.Stop()
				delete(m.tailers, serverID)
				log.Printf("Stopped log tailer for server %d (no subscribers)", serverID)
			}
			delete(m.clients, serverID)
		}
	}
}

// forwardLines forwards new log lines to all subscribed clients
func (m *LogStreamManager) forwardLines(serverID int64, tailer *collector.RawLogTailer) {
	for {
		select {
		case line, ok := <-tailer.Lines:
			if !ok {
				return // Tailer stopped
			}

			msg := LogMessage{
				Type:  "lines",
				Lines: []string{line},
			}
			data, _ := json.Marshal(msg)

			m.mu.RLock()
			clients := m.clients[serverID]
			for client := range clients {
				select {
				case client.send <- data:
				default:
					// Client buffer full, will be cleaned up
				}
			}
			m.mu.RUnlock()

		case err, ok := <-tailer.Errors:
			if !ok {
				return // Tailer stopped
			}
			log.Printf("Log tailer error for server %d: %v", serverID, err)
		}
	}
}

// handleLogWebSocket handles WebSocket connections for log streaming
func (r *Router) handleLogWebSocket(w http.ResponseWriter, req *http.Request) {
	// Validate auth from query parameter (WebSocket can't send headers on upgrade)
	token := req.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusUnauthorized, "token required")
		return
	}

	claims, err := r.auth.ValidateToken(token)
	if err != nil || claims == nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	// Get server ID
	serverIDStr := req.URL.Query().Get("server_id")
	serverID, err := strconv.ParseInt(serverIDStr, 10, 64)
	if err != nil || serverID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid server_id")
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Printf("Log WebSocket upgrade error: %v", err)
		return
	}

	client := &LogStreamClient{
		conn:    conn,
		send:    make(chan []byte, 256),
		manager: r.logStream,
	}

	// Subscribe to log stream
	initialLines, err := r.logStream.Subscribe(client, serverID)
	if err != nil {
		log.Printf("Log subscription error: %v", err)
		msg := LogMessage{Type: "error", Message: "failed to subscribe to logs"}
		data, _ := json.Marshal(msg)
		conn.WriteMessage(websocket.TextMessage, data)
		conn.Close()
		return
	}

	// Send initial lines
	if len(initialLines) > 0 {
		msg := LogMessage{Type: "initial", Lines: initialLines}
		data, _ := json.Marshal(msg)
		conn.WriteMessage(websocket.TextMessage, data)
	}

	// Start read/write pumps
	go client.writePump()
	go client.readPump()
}

// readPump reads messages from the WebSocket (handles close)
func (c *LogStreamClient) readPump() {
	defer func() {
		c.manager.Unsubscribe(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) {
				log.Printf("Log WebSocket error: %v", err)
			}
			break
		}
	}
}

// writePump sends messages to the WebSocket
func (c *LogStreamClient) writePump() {
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

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
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

// handleLogStatus returns whether logs are available for a server
func (r *Router) handleLogStatus(w http.ResponseWriter, req *http.Request) {
	serverID, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server ID")
		return
	}

	server, err := r.store.GetServerByID(req.Context(), serverID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"available": server.LogPath != "",
		"log_path":  server.LogPath,
	})
}
