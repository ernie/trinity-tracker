package api

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ernie/trinity-tools/internal/auth"
	"github.com/ernie/trinity-tools/internal/collector"
	"github.com/ernie/trinity-tools/internal/storage"
)

// Router holds the HTTP routes and dependencies
type Router struct {
	mux       *http.ServeMux
	store     *storage.Store
	manager   *collector.ServerManager
	wsHub     *WebSocketHub
	logStream *LogStreamManager
	auth      *auth.Service
	staticDir string
}

// NewRouter creates a new HTTP router
func NewRouter(store *storage.Store, manager *collector.ServerManager, authService *auth.Service, staticDir string) *Router {
	r := &Router{
		mux:       http.NewServeMux(),
		store:     store,
		manager:   manager,
		wsHub:     NewWebSocketHub(),
		logStream: NewLogStreamManager(store),
		auth:      authService,
		staticDir: staticDir,
	}

	// API routes
	r.mux.HandleFunc("GET /api/servers", r.handleGetServers)
	r.mux.HandleFunc("GET /api/servers/{id}", r.handleGetServer)
	r.mux.HandleFunc("GET /api/servers/{id}/status", r.handleGetServerStatus)
	r.mux.HandleFunc("GET /api/servers/{id}/players", r.handleGetServerPlayers)

	r.mux.HandleFunc("GET /api/players", r.handleGetPlayers)
	r.mux.HandleFunc("GET /api/players/verified", r.handleGetVerifiedPlayers)
	r.mux.HandleFunc("GET /api/players/{id}", r.handleGetPlayer)
	r.mux.HandleFunc("GET /api/players/{id}/stats", r.handleGetPlayerStatsByID)
	r.mux.HandleFunc("GET /api/players/{id}/matches", r.handleGetPlayerMatches)

	r.mux.HandleFunc("GET /api/matches", r.handleGetMatches)
	r.mux.HandleFunc("GET /api/matches/{id}", r.handleGetMatch)

	r.mux.HandleFunc("GET /api/stats/leaderboard", r.handleGetLeaderboard)

	// Auth routes
	r.mux.HandleFunc("POST /api/auth/login", r.handleLogin)
	r.mux.HandleFunc("POST /api/auth/logout", r.handleLogout)
	r.mux.HandleFunc("GET /api/auth/check", r.handleAuthCheck)
	r.mux.HandleFunc("POST /api/auth/change-password", r.requireAuth(r.handleChangePassword))

	// Account routes (authenticated users only)
	r.mux.HandleFunc("GET /api/account/profile", r.requireAuth(r.handleGetAccountProfile))
	r.mux.HandleFunc("POST /api/account/link-code", r.requireAuth(r.handleCreateLinkCode))

	// User management routes (admin only)
	r.mux.HandleFunc("GET /api/users", r.requireAdmin(r.handleListUsers))
	r.mux.HandleFunc("POST /api/users", r.requireAdmin(r.handleCreateUser))
	r.mux.HandleFunc("DELETE /api/users/{username}", r.requireAdmin(r.handleDeleteUser))
	r.mux.HandleFunc("PATCH /api/users/{id}", r.requireAdmin(r.handleUpdateUser))
	r.mux.HandleFunc("POST /api/users/{id}/reset-password", r.requireAdmin(r.handleResetUserPassword))

	// RCON routes (admin only)
	r.mux.HandleFunc("POST /api/servers/{id}/rcon", r.requireAdmin(r.handleRconCommand))
	r.mux.HandleFunc("GET /api/servers/{id}/rcon-status", r.handleRconStatus)

	// WebSocket endpoints
	r.mux.HandleFunc("GET /ws", r.handleWebSocket)
	r.mux.HandleFunc("GET /ws/logs", r.handleLogWebSocket)

	// Log status endpoint (admin only)
	r.mux.HandleFunc("GET /api/servers/{id}/log-status", r.requireAdmin(r.handleLogStatus))

	// Player management routes (admin only)
	r.mux.HandleFunc("GET /api/players/{id}/guids", r.handleGetPlayerGUIDs)
	r.mux.HandleFunc("POST /api/admin/players/{id}/merge", r.requireAdmin(r.handleMergePlayers))
	r.mux.HandleFunc("POST /api/admin/guids/{id}/split", r.requireAdmin(r.handleSplitGUID))

	// Health check
	r.mux.HandleFunc("GET /health", r.handleHealth)

	// Static files - only serve if staticDir is configured
	if staticDir != "" {
		r.mux.HandleFunc("GET /", r.handleStatic)
	}

	return r
}

// ServeHTTP implements http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// CORS headers for API
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if req.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	r.mux.ServeHTTP(w, req)
}

// StartWebSocketHub starts broadcasting events to WebSocket clients
func (r *Router) StartWebSocketHub() {
	go r.wsHub.Run()

	// Forward events from manager to WebSocket hub
	go func() {
		for event := range r.manager.Events() {
			r.wsHub.Broadcast(event)
		}
	}()
}

// handleStatic serves static files from the configured directory
// For SPA support, serves index.html for any path that doesn't match a file
func (r *Router) handleStatic(w http.ResponseWriter, req *http.Request) {
	// Clean the path
	path := filepath.Clean(req.URL.Path)
	if path == "/" {
		path = "/index.html"
	}

	// Construct full file path
	fullPath := filepath.Join(r.staticDir, path)

	// Security: ensure the path is within staticDir
	absStaticDir, _ := filepath.Abs(r.staticDir)
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absStaticDir) {
		http.NotFound(w, req)
		return
	}

	// Check if file exists
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		// SPA fallback: serve index.html for unknown paths
		fullPath = filepath.Join(r.staticDir, "index.html")
		info, err = os.Stat(fullPath)
		if err != nil {
			http.NotFound(w, req)
			return
		}
	}

	// Set content type based on extension
	contentType := getContentType(fullPath)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	// Serve the file
	http.ServeFile(w, req, fullPath)
}

// getContentType returns the content type for a file based on extension
func getContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/x-icon"
	default:
		return ""
	}
}

// Ensure fs.FS is imported for potential future use
var _ fs.FS
