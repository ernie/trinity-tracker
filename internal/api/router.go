package api

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ernie/trinity-tools/internal/auth"
	"github.com/ernie/trinity-tools/internal/collector"
	"github.com/ernie/trinity-tools/internal/domain"
	"github.com/ernie/trinity-tools/internal/storage"
)

// Router holds the HTTP routes and dependencies
type Router struct {
	mux          *http.ServeMux
	store        *storage.Store
	manager      *collector.ServerManager
	wsHub        *WebSocketHub
	logStream    *LogStreamManager
	auth         *auth.Service
	loginLimiter *rateLimiter
	staticDir    string
	quake3Dir    string
}

// NewRouter creates a new HTTP router
func NewRouter(store *storage.Store, manager *collector.ServerManager, authService *auth.Service, staticDir, quake3Dir string) *Router {
	r := &Router{
		mux:          http.NewServeMux(),
		store:        store,
		manager:      manager,
		wsHub:        NewWebSocketHub(),
		logStream:    NewLogStreamManager(store),
		auth:         authService,
		loginLimiter: newRateLimiter(15*time.Minute, 5),
		staticDir:    staticDir,
		quake3Dir:    quake3Dir,
	}

	// API routes
	r.mux.HandleFunc("GET /api/servers", r.handleGetServers)
	r.mux.HandleFunc("GET /api/servers/{id}", r.handleGetServer)
	r.mux.HandleFunc("GET /api/servers/{id}/status", r.handleGetServerStatus)
	r.mux.HandleFunc("GET /api/servers/{id}/players", r.handleGetServerPlayers)

	r.mux.HandleFunc("GET /api/players", r.handleGetPlayers)
	r.mux.HandleFunc("GET /api/players/{id}", r.handleGetPlayer)
	r.mux.HandleFunc("GET /api/players/{id}/stats", r.handleGetPlayerStatsByID)
	r.mux.HandleFunc("GET /api/players/{id}/matches", r.handleGetPlayerMatches)

	r.mux.HandleFunc("GET /api/matches", r.handleGetMatches)
	r.mux.HandleFunc("GET /api/matches/{id}", r.handleGetMatch)

	r.mux.HandleFunc("GET /api/stats/leaderboard", r.handleGetLeaderboard)

	// Auth routes
	r.mux.HandleFunc("POST /api/auth/login", r.rateLimit(r.loginLimiter, r.handleLogin))
	r.mux.HandleFunc("POST /api/auth/logout", r.handleLogout)
	r.mux.HandleFunc("GET /api/auth/check", r.handleAuthCheck)
	r.mux.HandleFunc("POST /api/auth/change-password", r.requireAuth(r.handleChangePassword))

	// Game auth (public - no JWT required)
	r.mux.HandleFunc("POST /api/auth/game-login", r.rateLimit(r.loginLimiter, r.handleGameLogin))

	// Game token management (requires JWT auth)
	r.mux.HandleFunc("GET /api/auth/game-token", r.requireAuth(r.handleGetGameToken))
	r.mux.HandleFunc("POST /api/auth/game-token", r.requireAuth(r.handleRotateGameToken))

	// Account routes (authenticated users only)
	r.mux.HandleFunc("GET /api/account/profile", r.requireAuth(r.handleGetAccountProfile))
	r.mux.HandleFunc("POST /api/account/link-code", r.requireAuth(r.handleCreateLinkCode))

	// Claim routes (player-initiated account creation)
	r.mux.HandleFunc("POST /api/claim/validate", r.handleClaimValidate)
	r.mux.HandleFunc("POST /api/claim/register", r.handleClaimRegister)
	r.mux.HandleFunc("POST /api/claim/link", r.requireAuth(r.handleClaimLink))

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
	r.mux.HandleFunc("GET /api/players/{id}/sessions", r.requireAdmin(r.handleGetPlayerSessions))
	r.mux.HandleFunc("POST /api/admin/players/{id}/merge", r.requireAdmin(r.handleMergePlayers))
	r.mux.HandleFunc("POST /api/admin/guids/{id}/split", r.requireAdmin(r.handleSplitGUID))
	r.mux.HandleFunc("GET /api/admin/sessions", r.requireAdmin(r.handleListAdminSessions))

	// Quake 3 file serving (admin only, for web game client)
	if quake3Dir != "" {
		r.mux.HandleFunc("GET /api/q3/{path...}", r.requireAdmin(r.handleQuake3File))
	}

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

// handleQuake3File serves Quake 3 game files to authenticated admin users
func (r *Router) handleQuake3File(w http.ResponseWriter, req *http.Request) {
	filePath := req.PathValue("path")
	if filePath == "" {
		http.NotFound(w, req)
		return
	}

	// Clean and construct full path
	cleaned := filepath.Clean("/" + filePath)
	fullPath := filepath.Join(r.quake3Dir, cleaned)

	// Security: ensure the path is within quake3Dir
	absQuake3Dir, _ := filepath.Abs(r.quake3Dir)
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absQuake3Dir+string(filepath.Separator)) && absPath != absQuake3Dir {
		http.NotFound(w, req)
		return
	}

	// Check if file exists and is not a directory
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, req)
		return
	}

	// Set content type for pk3 files
	if strings.HasSuffix(strings.ToLower(fullPath), ".pk3") {
		w.Header().Set("Content-Type", "application/octet-stream")
	}

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
	case ".wasm":
		return "application/wasm"
	case ".tvd":
		return "application/octet-stream"
	default:
		return ""
	}
}

// populateDemoURLs checks for demo files on disk and sets DemoURL for matches that have one
func (r *Router) populateDemoURLs(matches []domain.MatchSummary) {
	if r.staticDir == "" {
		return
	}
	for i := range matches {
		if matches[i].UUID == "" {
			continue
		}
		demoPath := filepath.Join(r.staticDir, "demos", matches[i].UUID+".tvd")
		if _, err := os.Stat(demoPath); err == nil {
			matches[i].DemoURL = "/demos/" + matches[i].UUID + ".tvd"
		}
	}
}

// Ensure fs.FS is imported for potential future use
var _ fs.FS
