package api

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/auth"
	"github.com/ernie/trinity-tracker/internal/collector"
	"github.com/ernie/trinity-tracker/internal/console"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
	"github.com/ernie/trinity-tracker/internal/natsbus"
	"github.com/ernie/trinity-tracker/internal/storage"
)

// liveKeyPattern bounds a server key to the charset the collector relay
// accepts; handleLive puts the key raw into a redirect Location.
var liveKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Router holds the HTTP routes and their dependencies.
type Router struct {
	mux           *http.ServeMux
	store         *storage.Store
	manager       *collector.ServerManager
	writer        *hub.Writer
	poller        *hub.RemotePoller
	wsHub         *WebSocketHub
	auth          *auth.Service
	loginLimiter  *rateLimiter
	rotateLimiter *rotationLimiter
	staticDir     string
	quake3Dir     string
	userProv      hub.UserProvisioner
	// RCON for a remote source goes over NATS through rconClient; RCON
	// for localSource goes straight to the in-process ServerManager.
	rconClient  *natsbus.RconClient
	localSource string
	// The registry holds the in-process collector's console rings; the
	// relay reaches remote sources.
	consoleReg   *console.Registry
	consoleRelay *consoleRelay
	// Test seam: replaces dispatchRcon's transport, since the real ones
	// need a live ServerManager or NATS.
	testRconDispatch func(server *domain.Server, command string, role natsbus.RconRole) (string, error)
	// Throttles pat.last_used_at writes to about one a minute per token,
	// so a chatty CLI session doesn't hammer SQLite.
	patTouchMu   sync.Mutex
	patTouchSeen map[int64]time.Time
	respCache    *responseCache
	version      string
}

// SetVersion records the running binary's version for /api/version.
func (r *Router) SetVersion(v string) {
	r.version = v
}

// SetPoller plugs in the hub's UDP poller, which the server status and
// players endpoints read for live state.
func (r *Router) SetPoller(p *hub.RemotePoller) {
	r.poller = p
}

// lookupServerStatus returns the poller's latest status for id, or nil
// when there is none or no poller is set.
func (r *Router) lookupServerStatus(id int64) *domain.ServerStatus {
	if r.poller == nil {
		return nil
	}
	return r.poller.GetServerStatus(id)
}

// SetUserProvisioner plugs in the NATS credentials backend. Without one,
// the credential endpoints return 501.
func (r *Router) SetUserProvisioner(p hub.UserProvisioner) {
	r.userProv = p
}

// NewRouter creates the HTTP router and registers its routes.
func NewRouter(store *storage.Store, manager *collector.ServerManager, writer *hub.Writer, authService *auth.Service, staticDir, quake3Dir string) *Router {
	r := &Router{
		mux:           http.NewServeMux(),
		store:         store,
		manager:       manager,
		writer:        writer,
		wsHub:         NewWebSocketHub(),
		auth:          authService,
		loginLimiter:  newRateLimiter(15*time.Minute, 5),
		rotateLimiter: newRotationLimiter(5, 24*time.Hour),
		staticDir:     staticDir,
		quake3Dir:     quake3Dir,
		patTouchSeen:  make(map[int64]time.Time),
		respCache:     newResponseCache(),
	}

	// API routes
	r.mux.HandleFunc("GET /api/version", r.handleGetVersion)
	r.mux.HandleFunc("GET /api/servers", r.handleGetServers)
	r.mux.HandleFunc("GET /api/servers/{id}", r.handleGetServer)
	r.mux.HandleFunc("GET /api/servers/{id}/status", r.handleGetServerStatus)
	r.mux.HandleFunc("GET /api/servers/{id}/players", r.handleGetServerPlayers)

	// Hot anonymous reads; live data arrives over the websocket, so the
	// short cache is invisible.
	r.mux.HandleFunc("GET /api/players", r.cached(15*time.Second, r.handleGetPlayers))
	r.mux.HandleFunc("GET /api/players/{id}", r.cached(15*time.Second, r.handleGetPlayer))
	r.mux.HandleFunc("GET /api/players/{id}/stats", r.cached(15*time.Second, r.handleGetPlayerStatsByID))
	r.mux.HandleFunc("GET /api/players/{id}/matches", r.cached(15*time.Second, r.handleGetPlayerMatches))

	r.mux.HandleFunc("GET /api/matches", r.cached(10*time.Second, r.handleGetMatches))
	r.mux.HandleFunc("GET /api/matches/{id}", r.cached(10*time.Second, r.handleGetMatch))
	r.mux.HandleFunc("GET /api/matches/featured", r.cached(10*time.Second, r.handleGetFeaturedMatches))

	r.mux.HandleFunc("GET /api/stats/leaderboard", r.cached(30*time.Second, r.handleGetLeaderboard))

	r.mux.HandleFunc("GET /api/sources", r.handleGetSourceNames)

	// Auth
	r.mux.HandleFunc("POST /api/auth/login", r.rateLimit(r.loginLimiter, r.handleLogin))
	r.mux.HandleFunc("POST /api/auth/logout", r.handleLogout)
	r.mux.HandleFunc("POST /api/auth/logout-all", r.requireAuth(r.handleLogoutAll))
	r.mux.HandleFunc("GET /api/auth/check", r.handleAuthCheck)
	r.mux.HandleFunc("POST /api/auth/change-password", r.requireAuth(r.handleChangePassword))

	// CLI personal access tokens
	r.mux.HandleFunc("POST /api/auth/cli-login", r.rateLimit(r.loginLimiter, r.handleCLILogin))
	r.mux.HandleFunc("DELETE /api/auth/cli-login", r.handleCLILogout)

	// CLI console, addressed by (source, key)
	r.mux.HandleFunc("GET /api/console/servers", r.requireAuth(r.handleConsoleServers))
	r.mux.HandleFunc("POST /api/console/rcon", r.requireAuth(r.handleConsoleRcon))
	r.mux.HandleFunc("GET /api/console/stream", r.requireAuth(r.handleConsoleStream))

	// Game client login and game tokens
	r.mux.HandleFunc("POST /api/auth/game-login", r.rateLimit(r.loginLimiter, r.handleGameLogin))
	r.mux.HandleFunc("GET /api/auth/game-token", r.requireAuth(r.handleGetGameToken))
	r.mux.HandleFunc("POST /api/auth/game-token", r.requireAuth(r.handleRotateGameToken))

	// Account
	r.mux.HandleFunc("GET /api/account/profile", r.requireAuth(r.handleGetAccountProfile))
	r.mux.HandleFunc("POST /api/account/link-code", r.requireAuth(r.handleCreateLinkCode))
	r.mux.HandleFunc("PATCH /api/account/featured-honor", r.requireAuth(r.handleSetFeaturedHonor))
	r.mux.HandleFunc("PATCH /api/account/portrait", r.requireAuth(r.handleSetPortrait))
	r.mux.HandleFunc("PATCH /api/account/display-name", r.requireAuth(r.handleUpdateDisplayName))

	// Claim: account creation from an in-game claim code
	r.mux.HandleFunc("POST /api/claim/validate", r.handleClaimValidate)
	r.mux.HandleFunc("POST /api/claim/register", r.handleClaimRegister)
	r.mux.HandleFunc("POST /api/claim/link", r.requireAuth(r.handleClaimLink))

	// User management
	r.mux.HandleFunc("GET /api/users", r.requireAdmin(r.handleListUsers))
	r.mux.HandleFunc("POST /api/users", r.requireAdmin(r.handleCreateUser))
	r.mux.HandleFunc("DELETE /api/users/{username}", r.requireAdmin(r.handleDeleteUser))
	r.mux.HandleFunc("PATCH /api/users/{id}", r.requireAdmin(r.handleUpdateUser))
	r.mux.HandleFunc("POST /api/users/{id}/reset-password", r.requireAdmin(r.handleResetUserPassword))

	// RCON; the handler authorizes by source ownership and per-server
	// admin delegation.
	r.mux.HandleFunc("POST /api/servers/{id}/rcon", r.requireAuth(r.handleRconCommand))
	r.mux.HandleFunc("GET /api/servers/{id}/rcon-status", r.handleRconStatus)

	r.mux.HandleFunc("GET /ws", r.handleWebSocket)

	// Asset fallbacks: nginx serves these paths from disk and sends
	// misses here through try_files, and the handler 404s or redirects
	// to the source that owns the asset.
	r.mux.HandleFunc("GET /demos/{filename}", r.handleDemo)
	// The bare /tv/{source}/{key} is the SPA player page, so the stream
	// needs its own suffix.
	r.mux.HandleFunc("GET /tv/{source}/{key}/stream", r.handleLive)
	r.mux.HandleFunc("GET /assets/levelshots/{filename}", r.handleLevelshot)
	r.mux.HandleFunc("GET /demopk3s/maps/{filename}", r.handleMapPk3)

	// Player management
	r.mux.HandleFunc("GET /api/players/{id}/guids", r.handleGetPlayerGUIDs)
	r.mux.HandleFunc("GET /api/players/{id}/sessions", r.requireAdmin(r.handleGetPlayerSessions))
	r.mux.HandleFunc("POST /api/admin/players/{id}/merge", r.requireAdmin(r.handleMergePlayers))
	r.mux.HandleFunc("POST /api/admin/guids/{id}/split", r.requireAdmin(r.handleSplitGUID))
	r.mux.HandleFunc("POST /api/admin/matches/{id}/feature", r.requireAdmin(r.handleFeatureMatch))
	r.mux.HandleFunc("DELETE /api/admin/matches/{id}/feature", r.requireAdmin(r.handleFeatureMatch))

	// Admin source management. A collector can publish nothing until
	// its source has a row.
	r.mux.HandleFunc("GET /api/admin/sources", r.requireAdmin(r.handleListApprovedSources))
	r.mux.HandleFunc("POST /api/admin/sources", r.requireAdmin(r.handleCreateSource))
	r.mux.HandleFunc("GET /api/admin/sources/pending", r.requireAdmin(r.handleListPendingSources))
	r.mux.HandleFunc("POST /api/admin/sources/{source}/approve", r.requireAdmin(r.handleApproveSource))
	r.mux.HandleFunc("POST /api/admin/sources/{source}/reject", r.requireAdmin(r.handleRejectSource))
	r.mux.HandleFunc("POST /api/admin/sources/{source}/rename", r.requireAdmin(r.handleRenamePendingSource))
	r.mux.HandleFunc("POST /api/admin/sources/{source}/deactivate", r.requireAdmin(r.handleDeactivateSource))
	r.mux.HandleFunc("POST /api/admin/sources/{source}/reactivate", r.requireAdmin(r.handleReactivateSource))
	r.mux.HandleFunc("GET /api/admin/sources/{source}/creds", r.requireAdmin(r.handleDownloadSourceCreds))
	r.mux.HandleFunc("POST /api/admin/sources/{source}/rotate-creds", r.requireAdmin(r.handleRotateSourceCreds))
	r.mux.HandleFunc("POST /api/admin/sources/{source}/owner", r.requireAdmin(r.handleTransferSourceOwner))
	r.mux.HandleFunc("GET /api/admin/sessions", r.requireAdmin(r.handleListAdminSessions))
	r.mux.HandleFunc("GET /api/admin/platform-stats", r.requireAdmin(r.handlePlatformStats))
	r.mux.HandleFunc("GET /api/admin/audit", r.requireAdmin(r.handleListAudit))

	// Owner self-service; per-source actions require the caller to own
	// the source.
	r.mux.HandleFunc("GET /api/sources/mine", r.requireAuth(r.handleGetMySources))
	r.mux.HandleFunc("POST /api/sources/request", r.requireAuth(r.handleRequestSource))
	r.mux.HandleFunc("GET /api/sources/mine/{source}/creds", r.requireAuth(r.handleDownloadMyCreds))
	r.mux.HandleFunc("POST /api/sources/mine/{source}/rotate-creds", r.requireAuth(r.handleRotateMyCreds))
	r.mux.HandleFunc("POST /api/sources/mine/{source}/leave", r.requireAuth(r.handleLeaveMySource))

	// Game files for the web client
	if quake3Dir != "" {
		r.mux.HandleFunc("GET /api/q3/{path...}", r.requireAdmin(r.handleQuake3File))
	}

	r.mux.HandleFunc("GET /health", r.handleHealth)

	if staticDir != "" {
		r.mux.HandleFunc("GET /", r.handleStatic)
	}

	return r
}

// ServeHTTP implements http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// The wildcard origin is safe with cookie auth: browsers never send
	// credentials to a wildcard origin. It keeps PAT scripting working
	// from other origins.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if req.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	r.mux.ServeHTTP(w, req)
}

// StartWebSocketHub starts broadcasting the in-process collector's
// events to WebSocket clients, with player IDs filled in by the writer.
func (r *Router) StartWebSocketHub() {
	go r.wsHub.Run()

	go func() {
		ctx := context.Background()
		for event := range r.manager.Events() {
			if r.writer != nil {
				event = r.writer.EnrichEvent(ctx, event)
			}
			r.Broadcast(event)
		}
	}()
}

// Broadcast sends an enriched event to every WebSocket client. It
// implements hub.LiveEventSink.
//
// This is the one handshake gate for live events: an event is dropped
// unless its server has been seen enforcing g_trinityHandshake. A zero
// ServerID means a misrouted envelope, since every emit site sets one.
// A nil writer, in tests, broadcasts everything.
func (r *Router) Broadcast(event domain.Event) {
	if r.writer == nil {
		r.wsHub.Broadcast(event)
		return
	}
	if event.ServerID == 0 {
		return
	}
	if !r.writer.IsHandshakeEnforced(context.Background(), event.ServerID) {
		return
	}
	r.wsHub.Broadcast(event)
}

// handleStatic serves the web bundle, with index.html for any path that
// isn't a file so the SPA can route it.
func (r *Router) handleStatic(w http.ResponseWriter, req *http.Request) {
	path := filepath.Clean(req.URL.Path)
	if path == "/" {
		path = "/index.html"
	}

	fullPath := filepath.Join(r.staticDir, path)

	absStaticDir, _ := filepath.Abs(r.staticDir)
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absStaticDir) {
		http.NotFound(w, req)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		fullPath = filepath.Join(r.staticDir, "index.html")
		info, err = os.Stat(fullPath)
		if err != nil {
			http.NotFound(w, req)
			return
		}
	}

	contentType := getContentType(fullPath)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	http.ServeFile(w, req, fullPath)
}

// handleQuake3File serves Quake III game files to admins.
func (r *Router) handleQuake3File(w http.ResponseWriter, req *http.Request) {
	filePath := req.PathValue("path")
	if filePath == "" {
		http.NotFound(w, req)
		return
	}

	cleaned := filepath.Clean("/" + filePath)
	fullPath := filepath.Join(r.quake3Dir, cleaned)

	absQuake3Dir, _ := filepath.Abs(r.quake3Dir)
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absQuake3Dir+string(filepath.Separator)) && absPath != absQuake3Dir {
		http.NotFound(w, req)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, req)
		return
	}

	if strings.HasSuffix(strings.ToLower(fullPath), ".pk3") {
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	http.ServeFile(w, req, fullPath)
}

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

// populateDemoURLs sets DemoURL only for matches with a recording, so
// the UI never shows a play button that leads nowhere.
func (r *Router) populateDemoURLs(matches []domain.MatchSummary) {
	for i := range matches {
		if matches[i].UUID != "" && matches[i].DemoAvailable {
			matches[i].DemoURL = "/demos/" + matches[i].UUID + ".tvd"
		}
	}
}

// handleDemo serves a recorded demo from disk, or redirects to the
// source that owns the recording.
func (r *Router) handleDemo(w http.ResponseWriter, req *http.Request) {
	uuid := stripSuffix(req.PathValue("filename"), ".tvd")
	if uuid == "" {
		http.NotFound(w, req)
		return
	}
	if r.staticDir != "" {
		local := filepath.Join(r.staticDir, "demos", uuid+".tvd")
		if info, err := os.Stat(local); err == nil && !info.IsDir() {
			// Demos are write-once under uuid names, so cache forever.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			http.ServeFile(w, req, local)
			return
		}
	}
	base, err := r.store.FindSourcePublicURLForDemo(req.Context(), uuid)
	if err != nil {
		log.Printf("handleDemo: %v", err)
		http.Error(w, "Lookup error.", http.StatusInternalServerError)
		return
	}
	if base == "" {
		http.NotFound(w, req)
		return
	}
	http.Redirect(w, req,
		strings.TrimRight(base, "/")+"/demos/"+uuid+".tvd",
		http.StatusFound)
}

// handleLive redirects a TrinityVision live stream request to the relay
// on the collector that owns the server, which serves it at /tv/<key>.
func (r *Router) handleLive(w http.ResponseWriter, req *http.Request) {
	source := req.PathValue("source")
	key := req.PathValue("key")
	if source == "" || !liveKeyPattern.MatchString(key) {
		http.NotFound(w, req)
		return
	}
	base, err := r.store.FindSourcePublicURL(req.Context(), source)
	if err != nil {
		log.Printf("handleLive: %v", err)
		http.Error(w, "Lookup error.", http.StatusInternalServerError)
		return
	}
	if base == "" {
		http.NotFound(w, req)
		return
	}
	http.Redirect(w, req,
		strings.TrimRight(base, "/")+"/tv/"+key,
		http.StatusFound)
}

// stripSuffix returns name without ext, or "" when name lacks ext or
// holds a path separator. ServeMux wildcards can't carry a literal
// suffix, so the asset handlers check it here.
func stripSuffix(name, ext string) string {
	if !strings.HasSuffix(name, ext) || strings.ContainsAny(name, "/\\") {
		return ""
	}
	return strings.TrimSuffix(name, ext)
}

// handleMapPk3 serves a demobaked map pk3 from disk, or redirects to a
// source that has the map. The path matches the engine loader's
// relative URL.
func (r *Router) handleMapPk3(w http.ResponseWriter, req *http.Request) {
	mapName := stripSuffix(req.PathValue("filename"), ".pk3")
	if mapName == "" {
		http.NotFound(w, req)
		return
	}
	if r.staticDir != "" {
		local := filepath.Join(r.staticDir, "demopk3s", "maps", mapName+".pk3")
		if info, err := os.Stat(local); err == nil && !info.IsDir() {
			http.ServeFile(w, req, local)
			return
		}
	}
	base, err := r.store.FindSourcePublicURLForMap(req.Context(), mapName)
	if err != nil {
		log.Printf("handleMapPk3: %v", err)
		http.Error(w, "Lookup error.", http.StatusInternalServerError)
		return
	}
	if base == "" {
		http.NotFound(w, req)
		return
	}
	http.Redirect(w, req,
		strings.TrimRight(base, "/")+"/demopk3s/maps/"+mapName+".pk3",
		http.StatusFound)
}

// handleLevelshot serves a map levelshot from disk, or redirects to a
// source that has the map.
func (r *Router) handleLevelshot(w http.ResponseWriter, req *http.Request) {
	mapName := stripSuffix(req.PathValue("filename"), ".jpg")
	if mapName == "" {
		http.NotFound(w, req)
		return
	}
	if r.staticDir != "" {
		local := filepath.Join(r.staticDir, "assets", "levelshots", mapName+".jpg")
		if info, err := os.Stat(local); err == nil && !info.IsDir() {
			// Rebuilt under the same names, so the cache lifetime is bounded.
			w.Header().Set("Cache-Control", "public, max-age=2592000")
			http.ServeFile(w, req, local)
			return
		}
	}
	base, err := r.store.FindSourcePublicURLForMap(req.Context(), mapName)
	if err != nil {
		log.Printf("handleLevelshot: %v", err)
		http.Error(w, "Lookup error.", http.StatusInternalServerError)
		return
	}
	if base == "" {
		http.NotFound(w, req)
		return
	}
	http.Redirect(w, req,
		strings.TrimRight(base, "/")+"/assets/levelshots/"+mapName+".jpg",
		http.StatusFound)
}

var _ fs.FS
