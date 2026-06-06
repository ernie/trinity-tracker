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
// accepts. handleLive concatenates the key raw into a redirect Location, so
// rejecting anything else (e.g. a decoded %2F, or other URL metacharacters) is
// defense-in-depth against a malformed 302.
var liveKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Router holds the HTTP routes and dependencies
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
	// rconClient and localSource together drive RCON dispatch:
	// requests for a remote source go via rconClient (NATS request/
	// reply), requests for localSource short-circuit through the
	// in-process ServerManager. See SetRconClient / SetLocalSource.
	rconClient  *natsbus.RconClient
	localSource string
	// Console following: registry for the in-process collector's rings,
	// relay for remote sources. See console_stream.go.
	consoleReg   *console.Registry
	consoleRelay *consoleRelay
	// testRconDispatch, when non-nil, replaces dispatchRcon's transport
	// step. Test seam only: the real transports are concrete types
	// (ServerManager, NATS RconClient) that need live infrastructure.
	testRconDispatch func(server *domain.Server, command string, role natsbus.RconRole) (string, error)
	// patTouch throttles pat.last_used_at writes (~1/min per token) so
	// a chatty CLI session doesn't write-amplify SQLite.
	patTouchMu   sync.Mutex
	patTouchSeen map[int64]time.Time
	// Version string surfaced to the frontend via /api/version. Set by
	// main.go after NewRouter so binary builds report their git-describe
	// tag; unset = "dev".
	version string
}

// SetVersion records the running binary's version for /api/version.
// Called once at startup from cmd/trinity/main.go.
func (r *Router) SetVersion(v string) {
	r.version = v
}

// SetPoller plugs in the hub's UDP poller. Always set in hub mode —
// the /api/servers/{id}/status and /players endpoints consult it for
// live state. Unset in collector-only mode (where no HTTP is served)
// and in rare hub-less test harnesses.
func (r *Router) SetPoller(p *hub.RemotePoller) {
	r.poller = p
}

// lookupServerStatus returns the most recent status the poller has
// for id, or nil. Wrapper so handlers are insulated from whether the
// poller is wired.
func (r *Router) lookupServerStatus(id int64) *domain.ServerStatus {
	if r.poller == nil {
		return nil
	}
	return r.poller.GetServerStatus(id)
}

// SetUserProvisioner plugs in a NATS cred-mint/revoke/download backend.
// main.go calls this with the embedded AuthStore in hub mode; when
// unset the /api/admin/sources/*/creds endpoints return 501.
func (r *Router) SetUserProvisioner(p hub.UserProvisioner) {
	r.userProv = p
}

// NewRouter creates a new HTTP router
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
	}

	// API routes
	r.mux.HandleFunc("GET /api/version", r.handleGetVersion)
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
	r.mux.HandleFunc("GET /api/matches/featured", r.handleGetFeaturedMatches)

	r.mux.HandleFunc("GET /api/stats/leaderboard", r.handleGetLeaderboard)

	// Public list of source names; powers the source-filter dropdown
	// in the activity log and matches list.
	r.mux.HandleFunc("GET /api/sources", r.handleGetSourceNames)

	// Auth routes
	r.mux.HandleFunc("POST /api/auth/login", r.rateLimit(r.loginLimiter, r.handleLogin))
	r.mux.HandleFunc("POST /api/auth/logout", r.handleLogout)
	r.mux.HandleFunc("POST /api/auth/logout-all", r.requireAuth(r.handleLogoutAll))
	r.mux.HandleFunc("GET /api/auth/check", r.handleAuthCheck)
	r.mux.HandleFunc("POST /api/auth/change-password", r.requireAuth(r.handleChangePassword))

	// CLI auth: PAT mint (password-verified, same limiter as login)
	// and self-revoke. See cli_login.go.
	r.mux.HandleFunc("POST /api/auth/cli-login", r.rateLimit(r.loginLimiter, r.handleCLILogin))
	r.mux.HandleFunc("DELETE /api/auth/cli-login", r.handleCLILogout)

	// Console: (source, key)-addressed control surface for the CLI.
	// See console.go and console_stream.go.
	r.mux.HandleFunc("GET /api/console/servers", r.requireAuth(r.handleConsoleServers))
	r.mux.HandleFunc("POST /api/console/rcon", r.requireAuth(r.handleConsoleRcon))
	r.mux.HandleFunc("GET /api/console/stream", r.requireAuth(r.handleConsoleStream))

	// Game auth (public - no JWT required)
	r.mux.HandleFunc("POST /api/auth/game-login", r.rateLimit(r.loginLimiter, r.handleGameLogin))

	// Game token management (requires JWT auth)
	r.mux.HandleFunc("GET /api/auth/game-token", r.requireAuth(r.handleGetGameToken))
	r.mux.HandleFunc("POST /api/auth/game-token", r.requireAuth(r.handleRotateGameToken))

	// Account routes (authenticated users only)
	r.mux.HandleFunc("GET /api/account/profile", r.requireAuth(r.handleGetAccountProfile))
	r.mux.HandleFunc("POST /api/account/link-code", r.requireAuth(r.handleCreateLinkCode))
	r.mux.HandleFunc("PATCH /api/account/featured-honor", r.requireAuth(r.handleSetFeaturedHonor))
	r.mux.HandleFunc("PATCH /api/account/display-name", r.requireAuth(r.handleUpdateDisplayName))

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

	// RCON routes. Auth-only (not admin-only); the handler
	// authorizes by source ownership + per-server admin delegation.
	r.mux.HandleFunc("POST /api/servers/{id}/rcon", r.requireAuth(r.handleRconCommand))
	r.mux.HandleFunc("GET /api/servers/{id}/rcon-status", r.handleRconStatus)

	// WebSocket endpoints
	r.mux.HandleFunc("GET /ws", r.handleWebSocket)

	// Asset fallbacks. nginx serves these paths as static when the file
	// is on disk; on a miss it forwards the request here via try_files
	// → @trinity_fallback. The handler then either 404s or 302s to the
	// source that owns the asset.
	r.mux.HandleFunc("GET /demos/{filename}", r.handleDemo)
	// 3-segment so the bare /tv/{source}/{key} stays the SPA player (a full nav
	// to it must serve the app, not download the relay stream).
	r.mux.HandleFunc("GET /tv/{source}/{key}/stream", r.handleLive)
	r.mux.HandleFunc("GET /assets/levelshots/{filename}", r.handleLevelshot)
	r.mux.HandleFunc("GET /demopk3s/maps/{filename}", r.handleMapPk3)

	// Player management routes (admin only)
	r.mux.HandleFunc("GET /api/players/{id}/guids", r.handleGetPlayerGUIDs)
	r.mux.HandleFunc("GET /api/players/{id}/sessions", r.requireAdmin(r.handleGetPlayerSessions))
	r.mux.HandleFunc("POST /api/admin/players/{id}/merge", r.requireAdmin(r.handleMergePlayers))
	r.mux.HandleFunc("POST /api/admin/guids/{id}/split", r.requireAdmin(r.handleSplitGUID))
	r.mux.HandleFunc("POST /api/admin/matches/{id}/feature", r.requireAdmin(r.handleFeatureMatch))
	r.mux.HandleFunc("DELETE /api/admin/matches/{id}/feature", r.requireAdmin(r.handleFeatureMatch))

	// Distributed-tracking source management. Sources are pre-provisioned:
	// POST /api/admin/sources creates a new source + mints initial creds
	// in one call. Collectors cannot publish anything (events, live
	// status, registration) until a row exists here.
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
	r.mux.HandleFunc("GET /api/admin/audit", r.requireAdmin(r.handleListAudit))

	// Owner-scoped self-service. GET /api/sources/mine returns the
	// caller's full source list (one card per source in My Servers);
	// the per-source action paths gate on owner_user_id == caller AND
	// the named source belonging to them.
	r.mux.HandleFunc("GET /api/sources/mine", r.requireAuth(r.handleGetMySources))
	r.mux.HandleFunc("POST /api/sources/request", r.requireAuth(r.handleRequestSource))
	r.mux.HandleFunc("GET /api/sources/mine/{source}/creds", r.requireAuth(r.handleDownloadMyCreds))
	r.mux.HandleFunc("POST /api/sources/mine/{source}/rotate-creds", r.requireAuth(r.handleRotateMyCreds))
	r.mux.HandleFunc("POST /api/sources/mine/{source}/leave", r.requireAuth(r.handleLeaveMySource))

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
	// CORS headers for API. The wildcard is safe alongside cookie
	// auth: browsers never send credentials to a wildcard origin, so
	// cookie-authed responses can't leak cross-site — and tightening
	// it would break PAT scripting from other origins.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if req.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	r.mux.ServeHTTP(w, req)
}

// StartWebSocketHub starts broadcasting events to WebSocket clients.
// Events flow: collector emits with GUIDs → writer enriches to fill
// player IDs → WebSocket hub broadcasts to browser clients.
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

// Broadcast forwards a pre-enriched event to all connected WebSocket
// clients, after dropping events for servers that haven't been
// observed enforcing g_trinityHandshake. Used by the in-process
// collector loop and by the hub-side live-event NATS subscriber so
// remote-collector activity appears on the unified dashboard in real
// time. *Router satisfies hub.LiveEventSink.
//
// The handshake gate is the single chokepoint: events for servers
// whose handshake_required column is 0 (or unknown) are dropped, as
// are events with a missing/unresolved ServerID — every legitimate
// emit site sets a non-zero ID, so a zero here means a misrouted or
// malformed envelope (e.g. natsbus.LiveSubscriber falling back to an
// unresolved RemoteServerID). The writer's in-memory cache makes this
// a hot-path read, falling back to a single SELECT on first
// observation per serverID. Production wiring always sets r.writer;
// the nil bypass exists for tests and early bring-up.
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

// populateDemoURLs sets DemoURL to a relative /demos/<uuid>.tvd for
// every match flagged demo_available. nginx + handleDemo handle the
// local-vs-remote dispatch on click. Empty DemoURL = no play button,
// so users don't see dead links for matches whose recording was
// discarded or never finalized.
func (r *Router) populateDemoURLs(matches []domain.MatchSummary) {
	for i := range matches {
		if matches[i].UUID != "" && matches[i].DemoAvailable {
			matches[i].DemoURL = "/demos/" + matches[i].UUID + ".tvd"
		}
	}
}

// handleDemo serves a recorded demo. Local file wins; otherwise 302 to
// the source that owns the match's recording. Only invoked from the
// nginx try_files fallback on misses.
func (r *Router) handleDemo(w http.ResponseWriter, req *http.Request) {
	uuid := stripSuffix(req.PathValue("filename"), ".tvd")
	if uuid == "" {
		http.NotFound(w, req)
		return
	}
	if r.staticDir != "" {
		local := filepath.Join(r.staticDir, "demos", uuid+".tvd")
		if info, err := os.Stat(local); err == nil && !info.IsDir() {
			http.ServeFile(w, req, local)
			return
		}
	}
	base, err := r.store.FindSourcePublicURLForDemo(req.Context(), uuid)
	if err != nil {
		log.Printf("handleDemo: %v", err)
		http.Error(w, "lookup error", http.StatusInternalServerError)
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

// handleLive 302-redirects a TrinityVision live request to the collector that
// owns the server. Unlike /demos there is never a local file and no extension —
// live is a stream (TVL1), not a .tvd recording; the bytes always come from the
// owning collector's relay listener (behind its nginx /tv/<key> location), so
// this is a pure router. The collector's relay serves the single-segment
// /tv/<key>; this /tv/<source>/<key>/stream route is the hub's stream entry
// point (the bare /tv/<source>/<key> renders the SPA player).
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
		http.Error(w, "lookup error", http.StatusInternalServerError)
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

// stripSuffix returns name without the given extension if it ends in
// it, otherwise the empty string. Helper for the asset handlers,
// which match {filename} and extract a name + validate the extension
// inline (Go ServeMux doesn't allow literal suffixes on wildcards).
func stripSuffix(name, ext string) string {
	if !strings.HasSuffix(name, ext) || strings.ContainsAny(name, "/\\") {
		return ""
	}
	return strings.TrimSuffix(name, ext)
}

// handleMapPk3 serves a demobaked map pk3. Local file wins; otherwise
// 302 to a source that's served the map. Wired under /demopk3s/maps/
// so the engine loader's relative URL keeps working.
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
		http.Error(w, "lookup error", http.StatusInternalServerError)
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

// handleLevelshot serves a map levelshot. Local file wins; otherwise
// 302 to a source that's served the map.
func (r *Router) handleLevelshot(w http.ResponseWriter, req *http.Request) {
	mapName := stripSuffix(req.PathValue("filename"), ".jpg")
	if mapName == "" {
		http.NotFound(w, req)
		return
	}
	if r.staticDir != "" {
		local := filepath.Join(r.staticDir, "assets", "levelshots", mapName+".jpg")
		if info, err := os.Stat(local); err == nil && !info.IsDir() {
			http.ServeFile(w, req, local)
			return
		}
	}
	base, err := r.store.FindSourcePublicURLForMap(req.Context(), mapName)
	if err != nil {
		log.Printf("handleLevelshot: %v", err)
		http.Error(w, "lookup error", http.StatusInternalServerError)
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

// Ensure fs.FS is imported for potential future use
var _ fs.FS
