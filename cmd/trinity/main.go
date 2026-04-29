// trinity - Quake 3 Arena statistics and tools
package main

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/ernie/trinity-tracker/cmd/trinity/setup"
	"github.com/ernie/trinity-tracker/internal/api"
	"github.com/ernie/trinity-tracker/internal/assets"
	"github.com/ernie/trinity-tracker/internal/auth"
	"github.com/ernie/trinity-tracker/internal/collector"
	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/hub"
	"github.com/ernie/trinity-tracker/internal/natsbus"
	"github.com/ernie/trinity-tracker/internal/storage"
	"github.com/nats-io/nats.go"
	"github.com/ftrvxmtrx/tga"
	flag "github.com/spf13/pflag"
	"golang.org/x/image/draw"
	"golang.org/x/term"
)

var version = "dev"

const defaultConfigPath = "/etc/trinity/config.yml"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		cmdInit(os.Args[2:])
	case "serve":
		cmdServe(os.Args[2:])
	case "server":
		cmdServer(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "players":
		cmdPlayers(os.Args[2:])
	case "matches":
		cmdMatches(os.Args[2:])
	case "leaderboard":
		cmdLeaderboard(os.Args[2:])
	case "user":
		cmdUser(os.Args[2:])
	case "levelshots":
		cmdLevelshots(os.Args[2:])
	case "portraits":
		cmdPortraits(os.Args[2:])
	case "medals":
		cmdMedals(os.Args[2:])
	case "skills":
		cmdSkills(os.Args[2:])
	case "assets":
		cmdAssets(os.Args[2:])
	case "demobake":
		cmdDemobake(os.Args[2:])
	case "version":
		fmt.Printf("trinity %s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: trinity <command> [options] [args]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  init [--no-systemd] [--dry-run]     Interactive install wizard (collector-only by default)")
	fmt.Println("  serve                               Start the stats server")
	fmt.Println("  server list                         Show configured game servers")
	fmt.Println("  server add [<key>] [--gametype X] [--port N] [flags]")
	fmt.Println("                                      Add a game server instance (interactive on a TTY)")
	fmt.Println("  server remove <key>                 Remove a game server instance")
	fmt.Println("  status                              Health checks + (hub mode) live game-server status")
	fmt.Println("  players [--humans]                  Show current players across all servers")
	fmt.Println("  matches [--recent N]                Show recent matches (default: 20)")
	fmt.Println("  leaderboard [--top N]               Show top players (default: 20)")
	fmt.Println("  user add [--admin] [--player-id N] <username>")
	fmt.Println("                                      Add a user (prompts for password)")
	fmt.Println("  user remove <username>              Remove a user")
	fmt.Println("  user list                           List all users")
	fmt.Println("  user reset <username>               Reset a user's password")
	fmt.Println("  user admin <username>               Toggle admin status for a user")
	fmt.Println("  levelshots [path]                   Extract levelshots from pk3 file(s)")
	fmt.Println("  portraits [path]                    Extract player portraits from pk3 file(s)")
	fmt.Println("  medals [path]                       Extract medal icons from pk3 file(s)")
	fmt.Println("  skills [path]                       Extract skill icons from pk3 file(s)")
	fmt.Println("  assets [path]                       Extract all assets (portraits, medals, skills, levelshots)")
	fmt.Println("  demobake [path]                     Build baseline pk3, map pk3s, and manifest for web demo playback")
	fmt.Println("  version                             Show version")
	fmt.Println("  help                                Show this help")
	fmt.Println()
	fmt.Println("Global Options:")
	fmt.Println("  --config <path>    Path to configuration file (default /etc/trinity/config.yml)")
	fmt.Println("  --url <url>        Base URL of the trinity server (default: derived from config)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  sudo trinity init                          # interactive setup")
	fmt.Println("  sudo trinity server add                    # interactive add (gametype, port, ...)")
	fmt.Println("  sudo trinity server add ffa --gametype ffa --port 27960")
	fmt.Println("  trinity serve --config /etc/trinity/config.yml")
	fmt.Println("  trinity players --humans")
	fmt.Println("  trinity matches --recent 50")
	fmt.Println("  trinity user add --admin myuser")
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config file")
	fs.Parse(args)

	cfgPath := *configPath
	if cfgPath == "" {
		if _, err := os.Stat(defaultConfigPath); err == nil {
			cfgPath = defaultConfigPath
		} else {
			log.Fatalf("No config file found at %s. Use --config to specify a config file.", defaultConfigPath)
		}
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Trinity %s starting...", version)
	log.Printf("Monitoring %d servers", len(cfg.Q3Servers))

	// Tracker is always non-nil after config.Load (absent block
	// defaults to hub+local-collector).
	hasHub := cfg.Tracker.Hub != nil
	hasCollector := cfg.Tracker.Collector != nil

	var store *storage.Store
	if hasHub {
		s, err := storage.New(cfg.Database.Path)
		if err != nil {
			log.Fatalf("Failed to initialize database: %v", err)
		}
		defer s.Close()
		store = s
		log.Printf("Database initialized at %s", cfg.Database.Path)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		writerOpts         []hub.Option
		ns                 *natsbus.Server
		subNC              *nats.Conn
		collectorNC        *nats.Conn
		subscriber         *natsbus.Subscriber
		rpcServer          *natsbus.RPCServer
		regSubscriber      *natsbus.RegistrationSubscriber
		registrar          *natsbus.Registrar
		remotePoller       *hub.RemotePoller
		bufferedPublisher  *natsbus.BufferedPublisher
		collectorRPC       *natsbus.RPCClient
		collectorSpill     *natsbus.SpillQueue
		livePublisher      *natsbus.LivePublisher
		liveSubscriber     *natsbus.LiveSubscriber
		collectorSource    string
		collectorWatermark *natsbus.WatermarkTracker
	)

	if hasHub {
		storeParent := filepath.Dir(cfg.Database.Path)
		var err error
		ns, err = natsbus.Start(cfg.Tracker, storeParent)
		if err != nil {
			log.Fatalf("Failed to start embedded NATS: %v", err)
		}
		log.Printf("Embedded NATS listening on %s (store=%s)", ns.ClientURL(), ns.StoreDir())
		subNC, err = ns.ConnectInternal(nats.Name("trinity-hub-subscriber"))
		if err != nil {
			log.Fatalf("Failed to connect hub subscriber NATS client: %v", err)
		}
		defer subNC.Close()
	}
	if hasCollector {
		opts := []nats.Option{nats.Name("trinity-collector")}
		var connURL string
		switch {
		case ns != nil:
			// Co-located: reuse hub-internal creds to skip per-source issuance.
			opts = append(opts, nats.InProcessServer(ns.NATSServer()), nats.UserCredentials(ns.Auth().InternalCredsPath()))
		default:
			creds := cfg.Tracker.NATS.CredentialsFile
			if creds == "" {
				log.Fatalf("tracker.nats.credentials_file is required in collector-only mode")
			}
			opts = append(opts,
				nats.UserCredentials(creds),
				nats.CustomInboxPrefix(natsbus.InboxPrefixFor(cfg.Tracker.Collector.SourceID)),
			)
			log.Printf("Collector using credentials file: %s", creds)
			connURL = cfg.Tracker.NATS.URL
			if connURL == "" {
				log.Fatalf("tracker.nats.url or tracker.collector.hub_host must be set in collector-only mode")
			}
		}
		var err error
		collectorNC, err = nats.Connect(connURL, opts...)
		if err != nil {
			log.Fatalf("Failed to connect collector NATS client: %v", err)
		}
		defer collectorNC.Close()
		if connURL != "" {
			log.Printf("Collector connected to NATS hub at %s", connURL)
		}
		collectorSource = cfg.Tracker.Collector.SourceID
		wm, err := natsbus.NewWatermarkTracker(cfg.Tracker.Collector.DataDir)
		if err != nil {
			log.Fatalf("Failed to load publish watermark: %v", err)
		}
		collectorWatermark = wm
		pub, err := natsbus.NewPublisherWithWatermark(collectorNC, collectorSource, wm.Current().LastSeq, wm)
		if err != nil {
			log.Fatalf("Failed to build fact-event publisher: %v", err)
		}
		spill, err := natsbus.NewSpillQueue(cfg.Tracker.Collector.DataDir)
		if err != nil {
			log.Fatalf("Failed to open publish spill queue: %v", err)
		}
		collectorSpill = spill
		bufPub := natsbus.NewBufferedPublisher(pub, natsbus.BufferedCapacity, spill)
		bufPub.Start(ctx)
		bufferedPublisher = bufPub
		rc, err := natsbus.NewRPCClient(collectorNC, collectorSource, 0)
		if err != nil {
			log.Fatalf("Failed to build RPC client: %v", err)
		}
		collectorRPC = rc
		log.Printf("Collector publishing as source=%s (last_seq=%d)", collectorSource, wm.Current().LastSeq)

		// Only tee live events onto NATS when no local hub is co-located
		// (otherwise manager.Events() → wsHub would double up).
		if !hasHub {
			lp, err := natsbus.NewLivePublisher(collectorNC, collectorSource)
			if err != nil {
				log.Fatalf("Failed to build live-event publisher: %v", err)
			}
			livePublisher = lp
		}
	}

	// collectorShutdown tears down outbound collector-side components.
	collectorShutdown := func() {
		if registrar != nil {
			registrar.Stop()
		}
		if bufferedPublisher != nil {
			bufferedPublisher.Stop()
		}
		if collectorSpill != nil {
			if err := collectorSpill.Close(); err != nil {
				log.Printf("spill queue close on shutdown: %v", err)
			}
		}
		if collectorWatermark != nil {
			if err := collectorWatermark.Flush(); err != nil {
				log.Printf("watermark flush on shutdown: %v", err)
			}
		}
	}

	if hasHub {
		writerOpts = append(writerOpts, hub.WithPreStop(func() {
			collectorShutdown()
			if liveSubscriber != nil {
				liveSubscriber.Stop()
			}
			if remotePoller != nil {
				remotePoller.Stop()
			}
			if subscriber != nil {
				subscriber.Stop()
			}
			if regSubscriber != nil {
				regSubscriber.Stop()
			}
			if rpcServer != nil {
				rpcServer.Stop()
			}
			if ns != nil {
				ns.Stop()
			}
		}))
	}

	var writer *hub.Writer
	if hasHub {
		writer = hub.NewWriter(store, writerOpts...)
		writer.Start(ctx)
		defer writer.Stop()

		// Auto-provision the local collector when hub+collector run in
		// the same process. Remote collectors are created by the admin
		// through POST /api/admin/sources; this branch handles the
		// local case so single-machine installs don't need a manual
		// provisioning step.
		if hasCollector && collectorSource != "" {
			if err := store.UpsertLocalSource(ctx, collectorSource); err != nil {
				log.Fatalf("Failed to register local source: %v", err)
			}
			writer.MarkSourceApproved(collectorSource)
		}
	}

	if hasHub {
		var err error
		subscriber, err = natsbus.NewSubscriber(subNC, writer)
		if err != nil {
			log.Fatalf("Failed to create fact-event subscriber: %v", err)
		}
		subscriber.Start(ctx)
		log.Printf("Hub subscribed to %s", natsbus.StreamEvents)

		rpcServer, err = natsbus.RegisterRPCHandlers(subNC, writer)
		if err != nil {
			log.Fatalf("Failed to register RPC handlers: %v", err)
		}
		log.Printf("Hub RPC handlers registered (queue group %s)", natsbus.RPCQueueGroup)

		regSubscriber, err = natsbus.NewRegistrationSubscriber(subNC, writer)
		if err != nil {
			log.Fatalf("Failed to subscribe to registrations: %v", err)
		}
		log.Printf("Hub subscribed to %s*", natsbus.RegistrationSubjectPrefix)

	}

	// Hub-side UDP poller: feeds live cards and /api/servers/{id}/status.
	if hasHub {
		remotePoller = hub.NewRemotePoller(store, collector.NewQ3Client(), cfg.Server.PollInterval, writer.Presence(), writer)
		remotePoller.Start(ctx)
		log.Printf("Hub polling every %v", cfg.Server.PollInterval)
	}

	// Route manager I/O: writer directly in hub-only; NATS RPC +
	// buffered publisher when a collector role is active.
	var (
		serverClient hub.ServerClient
		rpcClient    hub.RPCClient
		factPub      hub.FactPublisher
	)
	if hasCollector {
		serverClient = collectorRPC
		rpcClient = collectorRPC
		factPub = bufferedPublisher
	} else {
		serverClient = writer
		rpcClient = writer
		factPub = writer
	}
	manager := collector.NewServerManager(cfg, serverClient, rpcClient, factPub)
	if livePublisher != nil {
		manager.SetLivePublisher(livePublisher)
	}

	// Replay cutoff: the collector's NATS publisher watermark says
	// "I have already published everything up to this timestamp; treat
	// older log events as silent state-rebuild only." If no watermark
	// is available, the manager picks a per-server cutoff using the
	// log file's size as a fresh-vs-retrofit signal.
	if collectorWatermark != nil {
		if wm := collectorWatermark.Current(); !wm.LastTS.IsZero() {
			manager.SetReplayCutoff(wm.LastTS)
		}
	}

	if err := manager.Start(ctx); err != nil {
		log.Fatalf("Failed to start server manager: %v", err)
	}
	log.Printf("Server manager started, polling every %v", cfg.Server.PollInterval)

	// Start the heartbeat registrar after the manager so the initial
	// publish reflects the actual roster.
	if hasCollector {
		var err error
		registrar, err = natsbus.NewRegistrar(
			collectorNC,
			collectorSource,
			version,
			cfg.Tracker.Collector.PublicURL,
			manager.Roster,
			cfg.Tracker.Collector.HeartbeatInterval.D(),
		)
		if err != nil {
			log.Fatalf("Failed to build registrar: %v", err)
		}
		registrar.Start(ctx)
		log.Printf("Collector heartbeating every %v", cfg.Tracker.Collector.HeartbeatInterval.D())
	}

	// Collector-only mode: no HTTP UI, just wait for signal.
	if !hasHub {
		log.Printf("Running in collector-only mode; no HTTP server")
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("Received signal %v, shutting down...", sig)
		log.Println("Stopping server manager...")
		manager.Stop()
		collectorShutdown()
		cancel()
		log.Println("Shutdown complete")
		return
	}

	authService := auth.NewService(cfg.Auth.JWTSecret, cfg.Auth.TokenDuration)
	if cfg.Auth.JWTSecret == "" {
		log.Printf("Warning: No JWT secret configured. Auth tokens will use an empty secret.")
	}

	router := api.NewRouter(store, manager, writer, authService, cfg.Server.StaticDir, cfg.Server.Quake3Dir)
	if remotePoller != nil {
		router.SetPoller(remotePoller)
		remotePoller.SetSink(router)
	}
	if ns != nil {
		router.SetUserProvisioner(ns.Auth())
	}
	router.StartWebSocketHub()
	log.Printf("Serving static files from %s", cfg.Server.StaticDir)

	// Forward remote-collector live events onto the WebSocket hub.
	// selfSource skips the in-process collector's own events.
	if hasHub {
		ls, err := natsbus.NewLiveSubscriber(subNC, store, writer, router, collectorSource)
		if err != nil {
			log.Fatalf("Failed to start live subscriber: %v", err)
		}
		liveSubscriber = ls
		log.Printf("Hub subscribed to %s*", natsbus.SubjectLivePrefix)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.ListenAddr, cfg.Server.HTTPPort)
	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("HTTP server listening on %s", addr)
		log.Printf("Web UI available at http://%s", addr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case sig := <-sigCh:
		log.Printf("Received signal %v, shutting down...", sig)
	case err := <-serverErr:
		log.Fatalf("HTTP server error: %v", err)
	}

	log.Println("Shutting down HTTP server...")
	httpCtx, httpCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer httpCancel()
	if err := server.Shutdown(httpCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("Stopping server manager...")
	manager.Stop()

	cancel()
	log.Println("Shutdown complete")
}

// CLI helper variables
var (
	baseURL = "http://localhost:8080"
	dbPath  string
)

// loadCLIConfigFromFlags loads config using pre-parsed flag values
func loadCLIConfigFromFlags(configPath, url string) *config.Config {
	// Load config file
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load config from %s: %v\n", configPath, err)
		dbPath = "/var/lib/trinity/trinity.db"
		// Use explicit --url flag or default
		if url != "" {
			baseURL = url
		}
		return nil
	}

	// nil on collector-only; the hub-only CLI commands that need dbPath
	// will fail cleanly via storage.New("").
	if cfg.Database != nil {
		dbPath = cfg.Database.Path
	}
	// Derive URL from config, but allow --url flag to override
	if url != "" {
		baseURL = url
	} else {
		baseURL = fmt.Sprintf("http://%s:%d", cfg.Server.ListenAddr, cfg.Server.HTTPPort)
	}
	return cfg
}

func loadCLIConfig(args []string) (*config.Config, []string) {
	fs := flag.NewFlagSet("cli", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the trinity server")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, *url)
	return cfg, fs.Args()
}

// cmdStatus reports overall install health (config, service, DB,
// hub reachability, public URL, local HTTP). On a hub install where
// the local HTTP server is up, it also dumps the live in-game
// per-server table — the question "what's happening on my hub right
// now?" that the original `trinity status` answered.
//
// Exits 1 on any failed health check so it's scriptable.
func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the trinity server (overrides config)")
	fs.Parse(args)

	failures := 0
	pass := func(label, detail string) { fmt.Printf("  ✓ %-12s %s\n", label, detail) }
	fail := func(label, detail string) {
		fmt.Printf("  ✗ %-12s %s\n", label, detail)
		failures++
	}

	fmt.Printf("Trinity %s\n\n", version)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fail("Config", fmt.Sprintf("%s: %v", *configPath, err))
		fmt.Println("\n1 check failed.")
		os.Exit(1)
	}
	pass("Config", fmt.Sprintf("%s loaded, %d game servers", *configPath, len(cfg.Q3Servers)))

	if out, err := exec.Command("systemctl", "is-active", "trinity").Output(); err == nil {
		state := strings.TrimSpace(string(out))
		if state == "active" {
			pass("Service", "trinity.service is active")
		} else {
			fail("Service", fmt.Sprintf("trinity.service is %s (run: sudo systemctl start trinity)", state))
		}
	}
	// systemctl missing or failed → silent skip; many environments don't use it.

	if cfg.Database != nil && cfg.Database.Path != "" {
		if info, err := os.Stat(cfg.Database.Path); err == nil && !info.IsDir() {
			pass("Database", cfg.Database.Path)
		} else {
			fail("Database", fmt.Sprintf("%s missing — service has not started successfully yet", cfg.Database.Path))
		}
	}

	isCollector := cfg.Tracker != nil && cfg.Tracker.Collector != nil
	isHub := cfg.Tracker == nil || cfg.Tracker.Hub != nil

	if isCollector {
		creds := cfg.Tracker.NATS.CredentialsFile
		if creds != "" {
			if _, err := os.Stat(creds); err == nil {
				pass("Credentials", creds)
			} else {
				fail("Credentials", fmt.Sprintf("%s: %v", creds, err))
			}
		}
		if hub := cfg.Tracker.Collector.HubHost; hub != "" {
			u := "https://" + hub + "/health"
			if reachable(u) {
				pass("Hub", hub+" reachable")
			} else {
				fail("Hub", u+" not reachable — check DNS, firewall, hub status")
			}
		}
		if pub := cfg.Tracker.Collector.PublicURL; pub != "" {
			if reachable(pub) {
				pass("Public URL", pub+" reachable")
			} else {
				fail("Public URL", pub+" not reachable — nginx down or DNS not pointing here?")
			}
		}
	}

	httpReachable := false
	if isHub {
		listen := cfg.Server.ListenAddr
		if listen == "" || listen == "0.0.0.0" {
			listen = "127.0.0.1"
		}
		u := fmt.Sprintf("http://%s:%d/health", listen, cfg.Server.HTTPPort)
		if reachable(u) {
			pass("HTTP", u+" → 200")
			httpReachable = true
		} else {
			fail("HTTP", u+" not reachable — service running but not bound?")
		}
	}

	fmt.Println()
	if failures > 0 {
		fmt.Printf("%d check(s) failed.\n", failures)
	} else {
		fmt.Println("All checks passed.")
	}

	if isHub && httpReachable {
		fmt.Println()
		printLiveServerTable(*configPath, *url)
	}

	if failures > 0 {
		os.Exit(1)
	}
}

// printLiveServerTable hits the local trinity HTTP API for the
// per-server in-game state and renders a tabwriter table. Best-effort
// — failures here don't affect the health-check exit code.
func printLiveServerTable(configPath, urlOverride string) {
	loadCLIConfigFromFlags(configPath, urlOverride)

	var servers []map[string]interface{}
	if err := getJSON("/api/servers", &servers); err != nil {
		fmt.Fprintf(os.Stderr, "live status unavailable: %v\n", err)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVER\tMAP\tPLAYERS\tHUMANS\tSTATUS")
	fmt.Fprintln(w, "------\t---\t-------\t------\t------")
	for _, srv := range servers {
		idF, _ := srv["id"].(float64)
		id := int64(idF)
		name := jsonString(srv, "key")
		if name == "" {
			name = fmt.Sprintf("server-%d", id)
		}

		var status map[string]interface{}
		if err := getJSON(fmt.Sprintf("/api/servers/%d/status", id), &status); err != nil {
			fmt.Fprintf(w, "%s\t-\t-\t-\tOFFLINE\n", name)
			continue
		}

		mapName := jsonString(status, "map")
		if mapName == "" {
			mapName = "-"
		}
		players := 0
		if p, ok := status["players"].([]interface{}); ok {
			players = len(p)
		}
		humans, _ := status["human_count"].(float64)

		statusStr := "ONLINE"
		if online, ok := status["online"].(bool); ok && !online {
			statusStr = "OFFLINE"
		}

		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n", name, mapName, players, int(humans), statusStr)
	}
	w.Flush()
}

// jsonString safely pulls a string field from a decoded JSON map.
// Returns "" for missing or non-string fields so render code can
// substitute a placeholder rather than panic on type assertions.
func jsonString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// reachable does a HEAD with a short timeout. Returns true for any
// 2xx/3xx/4xx (the host is up and answering); only 5xx and transport
// errors count as a failure for liveness purposes.
func reachable(url string) bool {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

func cmdPlayers(args []string) {
	fs := flag.NewFlagSet("players", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the trinity server")
	humansOnly := fs.Bool("humans", false, "show only human players")
	fs.Parse(args)

	loadCLIConfigFromFlags(*configPath, *url)

	// Get servers
	var servers []map[string]interface{}
	if err := getJSON("/api/servers", &servers); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVER\tPLAYER\tSCORE\tPING\tTYPE")
	fmt.Fprintln(w, "------\t------\t-----\t----\t----")

	for _, srv := range servers {
		id := int64(srv["id"].(float64))
		name := srv["name"].(string)

		var status map[string]interface{}
		if err := getJSON(fmt.Sprintf("/api/servers/%d/status", id), &status); err != nil {
			continue
		}

		players, ok := status["players"].([]interface{})
		if !ok {
			continue
		}

		for _, player := range players {
			pm, ok := player.(map[string]interface{})
			if !ok {
				continue
			}

			isBot := pm["is_bot"].(bool)
			if *humansOnly && isBot {
				continue
			}

			playerType := "Human"
			if isBot {
				playerType = "Bot"
			}

			cleanName := pm["clean_name"].(string)
			score := int(pm["score"].(float64))
			ping := int(pm["ping"].(float64))

			fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n", name, cleanName, score, ping, playerType)
		}
	}

	w.Flush()
}

func cmdMatches(args []string) {
	fs := flag.NewFlagSet("matches", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the trinity server")
	limit := fs.Int("recent", 20, "number of recent matches to show")
	fs.Parse(args)

	loadCLIConfigFromFlags(*configPath, *url)

	var matches []map[string]interface{}
	if err := getJSON(fmt.Sprintf("/api/matches?limit=%d", *limit), &matches); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tMAP\tSTARTED\tENDED\tEXIT REASON")
	fmt.Fprintln(w, "--\t---\t-------\t-----\t-----------")

	for _, match := range matches {
		id := int64(match["id"].(float64))
		mapName := match["map_name"].(string)

		started := "-"
		if s, ok := match["started_at"].(string); ok {
			started = formatTime(s)
		}

		ended := "In Progress"
		if e, ok := match["ended_at"].(string); ok && e != "" {
			ended = formatTime(e)
		}

		exitReason := "-"
		if r, ok := match["exit_reason"].(string); ok && r != "" {
			exitReason = r
		}

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", id, mapName, started, ended, exitReason)
	}

	w.Flush()
}

func cmdLeaderboard(args []string) {
	fs := flag.NewFlagSet("leaderboard", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the trinity server")
	limit := fs.Int("top", 20, "number of top players to show")
	fs.Parse(args)

	loadCLIConfigFromFlags(*configPath, *url)

	var response map[string]interface{}
	if err := getJSON(fmt.Sprintf("/api/stats/leaderboard?limit=%d", *limit), &response); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	entries, ok := response["entries"].([]interface{})
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unexpected response format\n")
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RANK\tPLAYER\tFRAGS\tDEATHS\tK/D\tMATCHES")
	fmt.Fprintln(w, "----\t------\t-----\t------\t---\t-------")

	for i, entry := range entries {
		stat := entry.(map[string]interface{})
		player := stat["player"].(map[string]interface{})
		name := player["clean_name"].(string)
		frags := int64(stat["total_frags"].(float64))
		deaths := int64(stat["total_deaths"].(float64))
		kd := stat["kd_ratio"].(float64)
		matches := int64(stat["total_matches"].(float64))

		fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%.2f\t%d\n", i+1, name, frags, deaths, kd, matches)
	}

	w.Flush()
}

// cmdUser handles user subcommands
func cmdUser(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: user subcommand required: add, remove, list, reset, admin\n")
		os.Exit(1)
	}

	// For user commands, we need config but also the subcommand
	subCmd := args[0]
	cfg, remaining := loadCLIConfig(args[1:])
	_ = cfg // cfg may be nil if config loading failed

	// Open database
	store, err := storage.New(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	ctx := context.Background()

	switch subCmd {
	case "add":
		if err := cmdUserAdd(ctx, store, remaining); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "remove":
		if err := cmdUserRemove(ctx, store, remaining); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "list":
		if err := cmdUserList(ctx, store); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "reset":
		if err := cmdUserReset(ctx, store, remaining); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "admin":
		if err := cmdUserAdmin(ctx, store, remaining); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown user command: %s (use: add, remove, list, reset, admin)\n", subCmd)
		os.Exit(1)
	}
}

func cmdUserAdd(ctx context.Context, store *storage.Store, args []string) error {
	fs := flag.NewFlagSet("user add", flag.ExitOnError)
	isAdmin := fs.Bool("admin", false, "create as admin user")
	playerIDFlag := fs.Int64("player-id", 0, "link to player ID")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) < 1 {
		return fmt.Errorf("usage: trinity user add [--admin] [--player-id N] <username>")
	}

	username := remaining[0]
	var playerID *int64
	if *playerIDFlag != 0 {
		playerID = playerIDFlag
	}

	// Check if user already exists
	if _, err := store.GetUserByUsername(ctx, username); err == nil {
		return fmt.Errorf("user '%s' already exists", username)
	}

	// Check if player is already claimed
	if playerID != nil {
		claimed, err := store.IsPlayerClaimed(ctx, *playerID)
		if err != nil {
			return fmt.Errorf("failed to check player: %w", err)
		}
		if claimed {
			return fmt.Errorf("player %d is already linked to another user", *playerID)
		}
	}

	fmt.Print("Enter password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	fmt.Print("Confirm password: ")
	confirm, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	if string(password) != string(confirm) {
		return fmt.Errorf("passwords do not match")
	}

	hash, err := auth.HashPassword(string(password))
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := store.CreateUser(ctx, username, hash, *isAdmin, playerID); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	roleStr := "user"
	if *isAdmin {
		roleStr = "admin"
	}
	fmt.Printf("User '%s' created successfully (role: %s)\n", username, roleStr)
	return nil
}

func cmdUserRemove(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: trinity user remove <username>")
	}
	username := args[0]

	if err := store.DeleteUser(ctx, username); err != nil {
		return fmt.Errorf("failed to remove user: %w", err)
	}

	fmt.Printf("User '%s' removed\n", username)
	return nil
}

func cmdUserList(ctx context.Context, store *storage.Store) error {
	users, err := store.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list users: %w", err)
	}

	if len(users) == 0 {
		fmt.Println("No users configured")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "USERNAME\tROLE\tPLAYER_ID\tPWD_CHANGE\tLAST_LOGIN")
	fmt.Fprintln(w, "--------\t----\t---------\t----------\t----------")

	for _, user := range users {
		role := "user"
		if user.IsAdmin {
			role = "admin"
		}
		playerID := "-"
		if user.PlayerID != nil {
			playerID = fmt.Sprintf("%d", *user.PlayerID)
		}
		pwdChange := "no"
		if user.PasswordChangeRequired {
			pwdChange = "yes"
		}
		lastLogin := "never"
		if user.LastLogin != nil {
			lastLogin = user.LastLogin.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", user.Username, role, playerID, pwdChange, lastLogin)
	}
	return w.Flush()
}

func cmdUserReset(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: trinity user reset <username>")
	}
	username := args[0]

	user, err := store.GetUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("user not found: %s", username)
	}

	fmt.Print("Enter new password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	fmt.Print("Confirm password: ")
	confirm, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	if string(password) != string(confirm) {
		return fmt.Errorf("passwords do not match")
	}

	hash, err := auth.HashPassword(string(password))
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := store.ResetUserPassword(ctx, user.ID, hash); err != nil {
		return fmt.Errorf("failed to reset password: %w", err)
	}

	fmt.Printf("Password reset for '%s' (user will be required to change it on next login)\n", username)
	return nil
}

func cmdUserAdmin(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: trinity user admin <username>")
	}
	username := args[0]

	user, err := store.GetUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("user not found: %s", username)
	}

	newAdminStatus := !user.IsAdmin
	if err := store.UpdateUserAdmin(ctx, user.ID, newAdminStatus); err != nil {
		return fmt.Errorf("failed to update admin status: %w", err)
	}

	if newAdminStatus {
		fmt.Printf("User '%s' is now an admin\n", username)
	} else {
		fmt.Printf("User '%s' is no longer an admin\n", username)
	}
	return nil
}

// cmdLevelshots extracts levelshot images from pk3 files
func cmdLevelshots(args []string) {
	fs := flag.NewFlagSet("levelshots", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, "")
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config\n")
		os.Exit(1)
	}

	if cfg.Server.StaticDir == "" {
		fmt.Fprintf(os.Stderr, "Error: static_dir not configured in config file\n")
		os.Exit(1)
	}

	// Use remaining arg as path override, or default to quake3_dir from config
	remaining := fs.Args()
	inputPath := cfg.Server.Quake3Dir
	if len(remaining) > 0 {
		inputPath = remaining[0]
	}

	// Validate and create output directory
	outputDir := filepath.Join(cfg.Server.StaticDir, "assets", "levelshots")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create output directory %s: %v\n", outputDir, err)
		os.Exit(1)
	}

	pk3Files := collectPk3FilesOrdered(inputPath)
	if len(pk3Files) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no pk3 files found in %s\n", inputPath)
		os.Exit(1)
	}

	var totalExtracted int
	for _, pk3Path := range pk3Files {
		displayPath := pk3DisplayPath(pk3Path, inputPath)
		n, err := extractLevelshotsFromPk3(pk3Path, outputDir, displayPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", displayPath, err)
			continue
		}
		totalExtracted += n
	}

	fmt.Printf("Levelshots: %d extracted\n", totalExtracted)
}

// extractLevelshotsFromPk3 extracts levelshot images from a single pk3 file
func extractLevelshotsFromPk3(pk3Path, outputDir, displayPath string) (int, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return 0, fmt.Errorf("failed to open pk3: %w", err)
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		// Check if this is a levelshot
		lowerName := strings.ToLower(f.Name)
		if !strings.HasPrefix(lowerName, "levelshots/") {
			continue
		}

		// Extract map name and extension
		base := filepath.Base(f.Name)
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".jpg" && ext != ".tga" {
			continue
		}

		mapName := strings.TrimSuffix(base, filepath.Ext(base))
		mapName = strings.ToLower(mapName)

		// Output path is always .jpg
		outputPath := filepath.Join(outputDir, mapName+".jpg")

		// Extract and potentially convert
		if err := extractLevelshot(f, outputPath, ext); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to extract %s: %v\n", mapName, err)
			continue
		}

		fmt.Printf("  %s: %s\n", displayPath, mapName)
		extracted++
	}

	return extracted, nil
}

// extractLevelshot extracts a single levelshot, converting TGA to JPG if needed
func extractLevelshot(f *zip.File, outputPath, ext string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	var img image.Image
	if ext == ".jpg" {
		img, err = jpeg.Decode(rc)
	} else {
		img, err = tga.Decode(rc)
	}
	if err != nil {
		return fmt.Errorf("failed to decode %s: %w", ext, err)
	}

	// Resize to 640x480 using Catmull-Rom (bicubic) interpolation
	bounds := img.Bounds()
	if bounds.Dx() != 640 || bounds.Dy() != 480 {
		dst := image.NewRGBA(image.Rect(0, 0, 640, 480))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		img = dst
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}

	if err := jpeg.Encode(out, img, &jpeg.Options{Quality: 90}); err != nil {
		out.Close()
		return err
	}

	return out.Close()
}

// pk3DisplayPath returns a display-friendly path for a pk3 file relative to basePath
func pk3DisplayPath(pk3Path, basePath string) string {
	if rel, err := filepath.Rel(basePath, pk3Path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return filepath.Base(pk3Path)
}

func getJSON(path string, target interface{}) error {
	url := baseURL + path
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func formatTime(isoTime string) string {
	// Simple formatting - just show time portion
	if idx := strings.Index(isoTime, "T"); idx != -1 {
		time := isoTime[idx+1:]
		if dotIdx := strings.Index(time, "."); dotIdx != -1 {
			time = time[:dotIdx]
		}
		if zIdx := strings.Index(time, "Z"); zIdx != -1 {
			time = time[:zIdx]
		}
		return time
	}
	return isoTime
}

// cmdPortraits extracts player portrait icons from pk3 files
func cmdPortraits(args []string) {
	fs := flag.NewFlagSet("portraits", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, "")
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config\n")
		os.Exit(1)
	}

	if cfg.Server.StaticDir == "" {
		fmt.Fprintf(os.Stderr, "Error: static_dir not configured in config file\n")
		os.Exit(1)
	}

	// Use remaining arg as path override, or default to quake3_dir from config
	remaining := fs.Args()
	inputPath := cfg.Server.Quake3Dir
	if len(remaining) > 0 {
		inputPath = remaining[0]
	}

	outputDir := filepath.Join(cfg.Server.StaticDir, "assets", "portraits")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	pk3Files := collectPk3FilesOrdered(inputPath)
	if len(pk3Files) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no pk3 files found in %s\n", inputPath)
		os.Exit(1)
	}

	var totalExtracted int
	for _, pk3Path := range pk3Files {
		displayPath := pk3DisplayPath(pk3Path, inputPath)
		n, err := extractPortraitsFromPk3(pk3Path, outputDir, displayPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", displayPath, err)
			continue
		}
		totalExtracted += n
	}

	fmt.Printf("Portraits: %d extracted\n", totalExtracted)
}

// extractPortraitsFromPk3 extracts player portrait icons from a pk3 file
func extractPortraitsFromPk3(pk3Path, outputDir, displayPath string) (int, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return 0, fmt.Errorf("failed to open pk3: %w", err)
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		lowerName := strings.ToLower(f.Name)
		// Match models/players/<model>/icon_<skin>.tga
		if !strings.HasPrefix(lowerName, "models/players/") {
			continue
		}
		base := filepath.Base(f.Name)
		if !strings.HasPrefix(strings.ToLower(base), "icon_") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".tga" {
			continue
		}

		// Extract model name from path
		parts := strings.Split(f.Name, "/")
		if len(parts) < 4 {
			continue
		}

		var model string
		if strings.ToLower(parts[2]) == "heads" {
			// Team Arena heads: models/players/heads/<name>/icon_*.tga
			if len(parts) < 5 {
				continue
			}
			model = strings.ToLower(parts[3])
		} else {
			// Standard: models/players/<model>/icon_*.tga
			model = strings.ToLower(parts[2])
		}

		// Create model subdirectory
		modelDir := filepath.Join(outputDir, model)
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to create directory %s: %v\n", modelDir, err)
			continue
		}

		// Output path: portraits/<model>/icon_<skin>.png
		outputName := strings.TrimSuffix(strings.ToLower(base), ".tga") + ".png"
		outputPath := filepath.Join(modelDir, outputName)
		assetName := model + "/" + outputName

		if err := extractTgaToPng(f, outputPath, 128); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to extract %s: %v\n", f.Name, err)
			continue
		}

		fmt.Printf("  %s: %s\n", displayPath, assetName)
		extracted++
	}

	return extracted, nil
}

// cmdMedals extracts medal icons from pk3 files
func cmdMedals(args []string) {
	fs := flag.NewFlagSet("medals", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, "")
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config\n")
		os.Exit(1)
	}

	if cfg.Server.StaticDir == "" {
		fmt.Fprintf(os.Stderr, "Error: static_dir not configured in config file\n")
		os.Exit(1)
	}

	// Use remaining arg as path override, or default to quake3_dir from config
	remaining := fs.Args()
	inputPath := cfg.Server.Quake3Dir
	if len(remaining) > 0 {
		inputPath = remaining[0]
	}

	outputDir := filepath.Join(cfg.Server.StaticDir, "assets", "medals")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	pk3Files := collectPk3FilesOrdered(inputPath)
	if len(pk3Files) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no pk3 files found in %s\n", inputPath)
		os.Exit(1)
	}

	var totalExtracted int
	for _, pk3Path := range pk3Files {
		displayPath := pk3DisplayPath(pk3Path, inputPath)
		n, err := extractMedalsFromPk3(pk3Path, outputDir, displayPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", displayPath, err)
			continue
		}
		totalExtracted += n
	}

	fmt.Printf("Medals: %d extracted\n", totalExtracted)
}

// extractMedalsFromPk3 extracts medal icons from a pk3 file
func extractMedalsFromPk3(pk3Path, outputDir, displayPath string) (int, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return 0, fmt.Errorf("failed to open pk3: %w", err)
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		lowerName := strings.ToLower(f.Name)
		base := strings.ToLower(filepath.Base(f.Name))

		// Match menu/medals/medal_*.tga or ui/assets/medal_*.tga
		isMedalPath := (strings.HasPrefix(lowerName, "menu/medals/") || strings.HasPrefix(lowerName, "ui/assets/")) &&
			strings.HasPrefix(base, "medal_") &&
			strings.HasSuffix(base, ".tga")

		if !isMedalPath {
			continue
		}

		// Output: medals/medal_*.png (flat structure)
		outputName := strings.TrimSuffix(base, ".tga") + ".png"
		outputPath := filepath.Join(outputDir, outputName)

		if err := extractTgaToPng(f, outputPath, 128); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to extract %s: %v\n", f.Name, err)
			continue
		}

		fmt.Printf("  %s: %s\n", displayPath, outputName)
		extracted++
	}

	return extracted, nil
}

// cmdSkills extracts skill icons from pk3 files
func cmdSkills(args []string) {
	fs := flag.NewFlagSet("skills", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, "")
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config\n")
		os.Exit(1)
	}

	if cfg.Server.StaticDir == "" {
		fmt.Fprintf(os.Stderr, "Error: static_dir not configured in config file\n")
		os.Exit(1)
	}

	remaining := fs.Args()
	inputPath := cfg.Server.Quake3Dir
	if len(remaining) > 0 {
		inputPath = remaining[0]
	}

	outputDir := filepath.Join(cfg.Server.StaticDir, "assets", "skills")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	pk3Files := collectPk3FilesOrdered(inputPath)
	if len(pk3Files) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no pk3 files found in %s\n", inputPath)
		os.Exit(1)
	}

	var totalExtracted int
	for _, pk3Path := range pk3Files {
		displayPath := pk3DisplayPath(pk3Path, inputPath)
		n, err := extractSkillsFromPk3(pk3Path, outputDir, displayPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", displayPath, err)
			continue
		}
		totalExtracted += n
	}

	fmt.Printf("Skills: %d extracted\n", totalExtracted)
}

// extractSkillsFromPk3 extracts skill icons from a pk3 file
func extractSkillsFromPk3(pk3Path, outputDir, displayPath string) (int, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return 0, fmt.Errorf("failed to open pk3: %w", err)
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		lowerName := strings.ToLower(f.Name)
		base := strings.ToLower(filepath.Base(f.Name))

		// Match menu/art/skill[1-5].tga
		if !strings.HasPrefix(lowerName, "menu/art/") {
			continue
		}
		if !strings.HasPrefix(base, "skill") || !strings.HasSuffix(base, ".tga") {
			continue
		}
		// Verify it's skill1-5
		numPart := strings.TrimPrefix(base, "skill")
		numPart = strings.TrimSuffix(numPart, ".tga")
		if len(numPart) != 1 || numPart[0] < '1' || numPart[0] > '5' {
			continue
		}

		// Output: skills/skill[1-5].png
		outputName := strings.TrimSuffix(base, ".tga") + ".png"
		outputPath := filepath.Join(outputDir, outputName)

		if err := extractTgaToPng(f, outputPath, 128); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to extract %s: %v\n", f.Name, err)
			continue
		}

		fmt.Printf("  %s: %s\n", displayPath, outputName)
		extracted++
	}

	return extracted, nil
}

// cmdAssets runs all asset extraction commands
func cmdAssets(args []string) {
	fs := flag.NewFlagSet("assets", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, "")
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config\n")
		os.Exit(1)
	}

	remaining := fs.Args()
	inputPath := cfg.Server.Quake3Dir
	if len(remaining) > 0 {
		inputPath = remaining[0]
	}

	// Build args for sub-commands
	subArgs := []string{"--config", *configPath, inputPath}

	fmt.Println("=== Extracting Levelshots ===")
	cmdLevelshots(subArgs)
	fmt.Println()

	fmt.Println("=== Extracting Portraits ===")
	cmdPortraits(subArgs)
	fmt.Println()

	fmt.Println("=== Extracting Medals ===")
	cmdMedals(subArgs)
	fmt.Println()

	fmt.Println("=== Extracting Skills ===")
	cmdSkills(subArgs)
	fmt.Println()

	fmt.Println("=== All asset extraction complete ===")
}

// cmdDemobake builds baseline pk3s, manifest, and all map pk3s
func cmdDemobake(args []string) {
	fs := flag.NewFlagSet("demobake", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	output := fs.String("output", "", "output directory (default: {static_dir}/pk3s/)")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, "")
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config\n")
		os.Exit(1)
	}

	// Use remaining arg as quake3_dir override
	remaining := fs.Args()
	quake3Dir := cfg.Server.Quake3Dir
	if len(remaining) > 0 {
		quake3Dir = remaining[0]
	}

	outputDir := *output
	if outputDir == "" {
		if cfg.Server.StaticDir == "" {
			fmt.Fprintf(os.Stderr, "Error: static_dir not configured and --output not specified\n")
			os.Exit(1)
		}
		outputDir = filepath.Join(cfg.Server.StaticDir, "demopk3s")
	}

	if err := assets.BuildBaseline(quake3Dir, outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Demobake complete")
}


// dropPrivileges switches to the given service user. No-op if not root.
func dropPrivileges(username string) error {
	if os.Getuid() != 0 {
		return nil
	}
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("looking up user %s: %w", username, err)
	}
	gid, _ := strconv.Atoi(u.Gid)
	uid, _ := strconv.Atoi(u.Uid)
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("setgid: %w", err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("setuid: %w", err)
	}
	return nil
}

// serviceUser returns the service user from config, defaulting to "quake"
func serviceUser(cfg *config.Config) string {
	if cfg != nil && cfg.Server.ServiceUser != "" {
		return cfg.Server.ServiceUser
	}
	return "quake"
}

// useSystemd returns whether systemd integration is enabled
func useSystemd(cfg *config.Config) bool {
	if cfg != nil && cfg.Server.UseSystemd != nil {
		return *cfg.Server.UseSystemd
	}
	return detectSystemd()
}

// detectSystemd checks if the system is running systemd
func detectSystemd() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

// systemctlRun executes a systemctl command, printing stderr on failure
func systemctlRun(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// systemctlIsActive returns the active state of a systemd unit
func systemctlIsActive(unit string) string {
	out, err := exec.Command("systemctl", "is-active", unit).Output()
	if err != nil {
		return "not-found"
	}
	return strings.TrimSpace(string(out))
}

// writeEnvFile creates a server environment file
func writeEnvFile(path string, port int, game string) error {
	opts := fmt.Sprintf("+set net_port %d", port)
	if game != "" && game != "baseq3" {
		opts += fmt.Sprintf(" +set fs_game %s", game)
	}
	content := fmt.Sprintf("SERVER_OPTS=%s\n", opts)
	return os.WriteFile(path, []byte(content), 0644)
}

// readEnvFile parses a server environment file for port and game
func readEnvFile(path string) (port int, game string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "SERVER_OPTS=") {
			continue
		}
		opts := strings.TrimPrefix(line, "SERVER_OPTS=")
		parts := strings.Fields(opts)
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "+set" && i+2 < len(parts) {
				switch parts[i+1] {
				case "net_port":
					port, _ = strconv.Atoi(parts[i+2])
				case "fs_game":
					game = parts[i+2]
				}
			}
		}
		break
	}
	if game == "" {
		game = "baseq3"
	}
	return port, game, scanner.Err()
}

// cmdInit walks the operator through an interactive install of one
// of the three Trinity modes (hub+collector, hub-only, collector-only).
// On a TTY it prompts; without one it errors out and prints the
// flag set required to drive it from a script.
//
// Refuses to run when /etc/trinity/config.yml already exists. There
// is no --force: if the operator wants to re-init, they delete the
// file themselves — a deliberate act that protects against accidents.
func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	noSystemd := fs.Bool("no-systemd", false, "skip systemd unit installation")
	dryRun := fs.Bool("dry-run", false, "print what would happen instead of doing it (no root required)")
	// --allow-hub is intentionally undocumented in --help. Trinity's
	// front door is a collector that joins trinity.run; standing up
	// your own hub is an expert path covered in docs/distributed-deployment.md.
	allowHub := fs.Bool("allow-hub", false, "")
	_ = fs.MarkHidden("allow-hub")
	configPathFlag := fs.String("config", "/etc/trinity/config.yml", "destination config path")
	fs.Parse(args)

	// Dry-run is for previewing/testing: no host state changes, so no
	// root is needed and an existing config is fine to "re-plan" against.
	if !*dryRun && os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "Error: trinity init must be run as root (or use --dry-run).")
		os.Exit(1)
	}

	configPath := *configPathFlag
	if !*dryRun {
		if _, err := os.Stat(configPath); err == nil {
			fmt.Fprintf(os.Stderr, "Trinity is already initialized (%s exists).\n", configPath)
			fmt.Fprintln(os.Stderr, "To re-init, remove the config file first:")
			fmt.Fprintf(os.Stderr, "  sudo rm %s\n", configPath)
			os.Exit(1)
		}
	}

	if !setup.IsTTY() && !*dryRun {
		fmt.Fprintln(os.Stderr, "Error: trinity init is interactive — run it from a terminal.")
		fmt.Fprintln(os.Stderr, "For unattended/scripted installs, write /etc/trinity/config.yml directly")
		fmt.Fprintln(os.Stderr, "and use `trinity server add` to add servers afterward.")
		os.Exit(1)
	}

	useSd := !*noSystemd && detectSystemd()
	if *noSystemd {
		fmt.Fprintln(os.Stderr, "Note: --no-systemd passed; will not install or enable units.")
	} else if !useSd {
		fmt.Fprintln(os.Stderr, "Note: systemd not detected; will not install units.")
	}

	prompter := setup.NewStdPrompter()
	answers, err := setup.RunWizard(prompter, os.Stderr, *allowHub)
	if errors.Is(err, setup.ErrMissingPrereqs) {
		// The wizard already printed the "go get the creds file"
		// hint — exit non-zero (so install.sh sees the failure) but
		// with no extra noise.
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Wizard failed: %v\n", err)
		os.Exit(1)
	}

	if err := answers.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Answers invalid: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Review:")
	fmt.Fprintln(os.Stderr, "  Mode:        ", answers.Mode)
	fmt.Fprintln(os.Stderr, "  User:        ", answers.ServiceUser)
	fmt.Fprintln(os.Stderr, "  Listen:      ", fmt.Sprintf("%s:%d", answers.ListenAddr, answers.HTTPPort))
	if answers.HasHubFields() {
		fmt.Fprintln(os.Stderr, "  Database:    ", answers.DatabasePath)
		fmt.Fprintln(os.Stderr, "  Web assets:  ", answers.StaticDir)
	}
	if answers.RunsLocalServers() {
		fmt.Fprintln(os.Stderr, "  Quake3 dir:  ", answers.Quake3Dir)
		if answers.InstallEngine {
			fmt.Fprintln(os.Stderr, "  Engine:       latest from github.com/ernie/trinity-engine")
		}
	}
	if answers.Mode == setup.ModeCollector {
		fmt.Fprintln(os.Stderr, "  Hub:         ", answers.HubHost)
		fmt.Fprintln(os.Stderr, "  Public URL:  ", answers.PublicURL)
		fmt.Fprintln(os.Stderr, "  Source ID:   ", answers.SourceID)
		fmt.Fprintln(os.Stderr, "  Creds file:  ", answers.CredsFile)
	}
	for _, s := range answers.Servers {
		fmt.Fprintf(os.Stderr, "  Server:       %s (%s) :%d\n", s.Key, s.Gametype.Label(), s.Port)
	}
	fmt.Fprintln(os.Stderr)

	prompt := "Apply this configuration?"
	if *dryRun {
		prompt = "Show the plan?"
	}
	confirm, err := prompter.YesNo(prompt, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Read confirm: %v\n", err)
		os.Exit(1)
	}
	if !confirm {
		fmt.Fprintln(os.Stderr, "Aborted; nothing was written.")
		os.Exit(1)
	}

	err = setup.Apply(answers, setup.ApplyOptions{
		ConfigPath: configPath,
		UseSystemd: useSd,
		DryRun:     *dryRun,
		Out:        os.Stderr,
	})
	// Either way, drop the install-time staging dir install.sh handed
	// us via TRINITY_INIT_STAGE — Apply has either consumed the web/
	// payload or never will. Skip in dry-run so the operator can
	// re-run with confidence.
	if !*dryRun {
		setup.CleanupStage()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Apply failed: %v\n", err)
		os.Exit(1)
	}

	if *dryRun {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Dry run complete — no changes made. Re-run without --dry-run as root to apply.")
		return
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Done. Next steps:")
	step := 1
	if answers.RunsLocalServers() {
		needsBaseq3 := false
		needsMissionpack := false
		for _, s := range answers.Servers {
			if s.RunsMissionpack() {
				needsMissionpack = true
			} else {
				needsBaseq3 = true
			}
		}
		if needsBaseq3 {
			fmt.Fprintf(os.Stderr, "  %d. Place retail pak0.pk3 at %s/baseq3/pak0.pk3\n", step, answers.Quake3Dir)
			step++
		}
		if needsMissionpack {
			fmt.Fprintf(os.Stderr, "  %d. Place Team Arena's pak0.pk3 at %s/missionpack/pak0.pk3\n", step, answers.Quake3Dir)
			step++
		}
	}
	if useSd {
		fmt.Fprintf(os.Stderr, "  %d. Start: sudo systemctl start trinity.service", step)
		if answers.RunsLocalServers() {
			fmt.Fprint(os.Stderr, " quake3-servers.target")
		}
		fmt.Fprintln(os.Stderr)
		step++
	}
	if answers.Mode == setup.ModeCollector {
		fmt.Fprintf(os.Stderr, "  %d. Required: generate the levelshot images and demo-playback pk3s the\n", step)
		fmt.Fprintln(os.Stderr, "     hub serves to web viewers. Re-run when you add new maps.")
		// Absolute path because Fedora doesn't put /usr/local/bin on
		// root's default PATH, and `sudo -u` uses the target user's PATH
		// which on locked-down systems may also exclude /usr/local/bin.
		fmt.Fprintf(os.Stderr, "       sudo -u %s /usr/local/bin/trinity levelshots\n", answers.ServiceUser)
		fmt.Fprintf(os.Stderr, "       sudo -u %s /usr/local/bin/trinity demobake\n", answers.ServiceUser)
		step++
	}
}

// cmdServer dispatches server subcommands
func cmdServer(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: server subcommand required: list, add, remove\n")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		cmdServerList(args[1:])
	case "add":
		cmdServerAdd(args[1:])
	case "remove":
		cmdServerRemove(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown server command: %s (use: list, add, remove)\n", args[0])
		os.Exit(1)
	}
}

// cmdServerList shows configured game servers with optional systemd status
func cmdServerList(args []string) {
	fs := flag.NewFlagSet("server list", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(cfg.Q3Servers) == 0 {
		fmt.Println("No servers configured")
		return
	}

	useSd := useSystemd(cfg)
	configDir := filepath.Dir(*configPath)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if useSd {
		fmt.Fprintln(w, "NAME\tPORT\tGAME\tSERVICE\tSTATUS")
	} else {
		fmt.Fprintln(w, "NAME\tPORT\tGAME")
	}

	for _, srv := range cfg.Q3Servers {
		// Extract port from address
		port := ""
		if parts := strings.SplitN(srv.Address, ":", 2); len(parts) == 2 {
			port = parts[1]
		}

		// Try to read game from env file
		serverName := strings.ToLower(srv.Key)
		game := "baseq3"
		envPath := filepath.Join(configDir, serverName+".env")
		if envPort, envGame, err := readEnvFile(envPath); err == nil {
			game = envGame
			if port == "" {
				port = strconv.Itoa(envPort)
			}
		}

		if useSd {
			unit := "quake3-server@" + serverName
			status := systemctlIsActive(unit)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", srv.Key, port, game, unit, status)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\n", srv.Key, port, game)
		}
	}
	w.Flush()
}

// nextAvailablePort finds the lowest unused port >= 27960 based on existing config entries and env files
func nextAvailablePort(cfg *config.Config, configDir string) int {
	used := make(map[int]bool)

	// Scan config entries for ports in addresses
	for _, srv := range cfg.Q3Servers {
		if parts := strings.SplitN(srv.Address, ":", 2); len(parts) == 2 {
			if p, err := strconv.Atoi(parts[1]); err == nil {
				used[p] = true
			}
		}
	}

	// Scan env files
	entries, _ := os.ReadDir(configDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".env") {
			if p, _, err := readEnvFile(filepath.Join(configDir, e.Name())); err == nil && p > 0 {
				used[p] = true
			}
		}
	}

	for port := 27960; ; port++ {
		if !used[port] {
			return port
		}
	}
}

// cmdServerAdd adds a new game server instance. With no name and on
// a TTY, drops into the wizard's per-server prompt loop (gametype +
// port + RCON + log path), writes the .env file (port + +exec the
// shared <stem>.cfg) and the shared <stem>.cfg + rotation.<stem> if
// they don't already exist, and enables the systemd template instance.
// With a name and explicit flags, runs non-interactively for scripts.
func cmdServerAdd(args []string) {
	fs := flag.NewFlagSet("server add", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	port := fs.Int("port", 0, "server port (default: next available)")
	gametypeFlag := fs.String("gametype", "", "gametype: ffa, tournament, tdm, ctf, oneflag, overload, harvester")
	taFlag := fs.Bool("ta", false, "run under fs_game=missionpack (Team Arena weapons + maps); only meaningful for tdm/ctf")
	rconPassword := fs.String("rcon-password", "", "RCON password (default: generate)")
	logPath := fs.String("log-path", "", "log file path")
	fs.Parse(args)

	remaining := fs.Args()
	interactive := len(remaining) == 0 && setup.IsTTY()
	if len(remaining) < 1 && !interactive {
		fmt.Fprintln(os.Stderr, "Usage: trinity server add [<key>] [--gametype X] [--port N] [--rcon-password P] [--log-path P]")
		fmt.Fprintln(os.Stderr, "       (or run with no args on a TTY for an interactive prompt)")
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	configDir := filepath.Dir(*configPath)
	sysUser := serviceUser(cfg)
	useSd := useSystemd(cfg)

	answers := answersFromConfig(cfg)
	var s setup.ServerAnswers
	if interactive {
		fmt.Fprintln(os.Stderr, "Add a q3 server.")
		s, err = setup.PromptServer(setup.NewStdPrompter(), answers, len(cfg.Q3Servers))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Wizard failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		s, err = serverAnswersFromFlags(remaining[0], *gametypeFlag, *taFlag, *port, *rconPassword, *logPath, cfg, configDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	// Reject duplicates after collection — the wizard accepts any key
	// the operator types; we enforce uniqueness here against the live
	// config (rather than re-prompting in the wizard, which would
	// require it to know about existing config state).
	for _, existing := range cfg.Q3Servers {
		if strings.EqualFold(existing.Key, s.Key) {
			fmt.Fprintf(os.Stderr, "Error: server '%s' already exists\n", s.Key)
			os.Exit(1)
		}
	}

	uid, gid := uidGid(sysUser)

	// Root-only steps first (systemd, file mode/ownership we couldn't
	// set after privilege drop).
	if useSd && os.Getuid() == 0 {
		unit := "quake3-server@" + s.Key
		if err := systemctlRun("enable", unit); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: systemctl enable %s failed: %v\n", unit, err)
		} else {
			fmt.Println("Enabled:", unit)
		}
	}

	stem := setup.Stem(s.Gametype, s.UseMissionpack)

	// Write env file (in /etc/trinity, root-owned but group-readable
	// by the service user; matches what `trinity init` writes).
	envPath := filepath.Join(configDir, s.Key+".env")
	opts := fmt.Sprintf("+set net_port %d", s.Port)
	if s.RunsMissionpack() {
		opts += " +set fs_game missionpack"
	}
	opts += " +exec " + stem + ".cfg"
	if err := os.WriteFile(envPath, []byte(fmt.Sprintf("SERVER_OPTS=%s\n", opts)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing env file: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Wrote:", envPath)

	// Shared <stem>.cfg + rotation.<stem>: only write if absent (other
	// servers of this gametype already installed them).
	if cfg.Server.Quake3Dir != "" {
		cfgPath := filepath.Join(cfg.Server.Quake3Dir, s.ModFolder(), stem+".cfg")
		if _, err := os.Stat(cfgPath); err == nil {
			fmt.Printf("  NOTE: %s already exists — left alone.\n", cfgPath)
		} else if body, rerr := setup.RenderServerCfg(s.Gametype, s.UseMissionpack); rerr != nil {
			fmt.Fprintf(os.Stderr, "Warning: cfg template render: %v\n", rerr)
		} else if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err == nil {
			if err := os.WriteFile(cfgPath, []byte(body), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", cfgPath, err)
			} else {
				_ = os.Chown(cfgPath, uid, gid)
				fmt.Println("Wrote:", cfgPath)
			}
		}

		rotPath := filepath.Join(cfg.Server.Quake3Dir, s.ModFolder(), "rotation."+stem)
		if _, err := os.Stat(rotPath); err == nil {
			// rotation already in place; nothing to do.
		} else if body, rerr := setup.RenderRotation(s.Gametype, s.UseMissionpack); rerr != nil {
			fmt.Fprintf(os.Stderr, "Warning: rotation render: %v\n", rerr)
		} else if err := os.WriteFile(rotPath, body, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", rotPath, err)
		} else {
			_ = os.Chown(rotPath, uid, gid)
			fmt.Println("Wrote:", rotPath)
		}
	}

	// Append to config.yml (after side files, so a config save failure
	// doesn't leave us inconsistent — env+cfg without a config entry
	// is benign; reverse is harder to debug).
	server := config.Q3Server{
		Key:          s.Key,
		Address:      s.Address,
		LogPath:      s.LogPath,
		RconPassword: s.RconPassword,
	}
	config.AddServer(cfg, server)
	if err := config.Save(*configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Updated:", *configPath)

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Restart trinity:    sudo systemctl restart trinity")
	if useSd {
		fmt.Printf("  2. Start this server:  sudo systemctl start quake3-server@%s\n", s.Key)
	}
}

// answersFromConfig builds a minimal *setup.Answers from the live
// config so PromptServer can derive defaults (PublicURL → address
// host, etc.) without re-prompting the operator.
func answersFromConfig(cfg *config.Config) *setup.Answers {
	a := &setup.Answers{Quake3Dir: cfg.Server.Quake3Dir}
	if cfg.Tracker != nil && cfg.Tracker.Collector != nil {
		a.PublicURL = cfg.Tracker.Collector.PublicURL
	}
	// Just the keys — PromptServer's suggestKey uses them for
	// collision avoidance. Other fields aren't read.
	for _, srv := range cfg.Q3Servers {
		a.Servers = append(a.Servers, setup.ServerAnswers{Key: srv.Key})
	}
	return a
}

// serverAnswersFromFlags builds a ServerAnswers from CLI flags for
// the non-interactive code path. Defaults mirror what PromptServer
// would suggest.
func serverAnswersFromFlags(name, gametypeName string, ta bool, port int, rcon, logPath string, cfg *config.Config, configDir string) (setup.ServerAnswers, error) {
	s := setup.ServerAnswers{Key: strings.ToLower(name)}
	gt, err := parseGametype(gametypeName)
	if err != nil {
		return s, err
	}
	s.Gametype = gt
	if ta && (gt == setup.GametypeTDM || gt == setup.GametypeCTF) {
		s.UseMissionpack = true
	}
	s.Port = port
	if s.Port == 0 {
		s.Port = nextAvailablePort(cfg, configDir)
	}
	host := "127.0.0.1"
	if cfg.Tracker != nil && cfg.Tracker.Collector != nil && cfg.Tracker.Collector.PublicURL != "" {
		// Reuse the wizard's URL → host extractor so collector hosts
		// get a routable address by default.
		if h := setup.HostFromURL(cfg.Tracker.Collector.PublicURL); h != "" {
			host = h
		}
	}
	s.Address = fmt.Sprintf("%s:%d", host, s.Port)
	s.RconPassword = rcon
	if s.RconPassword == "" {
		s.RconPassword = setup.GenerateRCONPassword()
	}
	s.LogPath = logPath
	if s.LogPath == "" {
		s.LogPath = filepath.Join("/var/log/quake3", strings.ToLower(s.Key)+".log")
	}
	return s, nil
}

// parseGametype maps the operator-facing flag value to a Gametype.
// Empty defaults to FFA so old `trinity server add NAME` calls keep
// working with sensible defaults.
func parseGametype(name string) (setup.Gametype, error) {
	switch strings.ToLower(name) {
	case "", "ffa":
		return setup.GametypeFFA, nil
	case "tournament", "1v1":
		return setup.GametypeTournament, nil
	case "tdm":
		return setup.GametypeTDM, nil
	case "ctf":
		return setup.GametypeCTF, nil
	case "oneflag":
		return setup.GametypeOneFlag, nil
	case "overload":
		return setup.GametypeOverload, nil
	case "harvester":
		return setup.GametypeHarvester, nil
	}
	return 0, fmt.Errorf("unknown gametype %q (try ffa, tournament, tdm, ctf, oneflag, overload, harvester)", name)
}

// uidGid resolves a username to numeric ids; returns 0/0 if lookup
// fails so chowns degrade to "owned by root" rather than crashing.
func uidGid(name string) (int, int) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, 0
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return uid, gid
}

// cmdServerRemove removes a game server instance
func cmdServerRemove(args []string) {
	fs := flag.NewFlagSet("server remove", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: trinity server remove <name>\n")
		os.Exit(1)
	}

	name := strings.ToLower(remaining[0])

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	sysUser := serviceUser(cfg)
	useSd := useSystemd(cfg)
	configDir := filepath.Dir(*configPath)

	// Do root-only operations first
	if useSd && os.Getuid() == 0 {
		unit := "quake3-server@" + name
		fmt.Printf("Stopping %s...\n", unit)
		systemctlRun("stop", unit)
		fmt.Printf("Disabling %s...\n", unit)
		systemctlRun("disable", unit)
	}

	// Drop privileges for file I/O
	if err := dropPrivileges(sysUser); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to drop privileges: %v\n", err)
	}

	// Archive env file rather than deleting it. Operators sometimes
	// remove a server intending to add it back with a tweak; the
	// archived .env is a quick reference and a safety net for typos.
	envPath := filepath.Join(configDir, name+".env")
	if _, err := os.Stat(envPath); err == nil {
		stamp := time.Now().UTC().Format("20060102-150405")
		archived := envPath + ".removed-" + stamp
		if err := os.Rename(envPath, archived); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to archive %s: %v\n", envPath, err)
		} else {
			fmt.Println("Archived:", archived)
		}
	}

	// Remove from config (try both the raw name and uppercase as display name)
	found := config.RemoveServerByKey(cfg, name)
	if !found {
		found = config.RemoveServerByKey(cfg, strings.ToUpper(name))
	}
	if !found {
		// Try case-insensitive match
		for _, srv := range cfg.Q3Servers {
			if strings.EqualFold(srv.Key, name) {
				found = config.RemoveServerByKey(cfg, srv.Key)
				break
			}
		}
	}

	if found {
		if err := config.Save(*configPath, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Config: %s updated\n", *configPath)
	} else {
		fmt.Fprintf(os.Stderr, "Warning: no matching server entry found in config\n")
	}

	fmt.Println()
	fmt.Println("Restart trinity to apply: sudo systemctl restart trinity")
}

// collectPk3FilesOrdered returns pk3 files in Quake 3 load order (later files override earlier)
// Order: pak0-9 numerically, then remaining pk3s alphabetically
// Applied to baseq3 first, then missionpack
func collectPk3FilesOrdered(quake3Dir string) []string {
	var files []string

	// Check if quake3Dir is a single file
	info, err := os.Stat(quake3Dir)
	if err != nil {
		return files
	}
	if !info.IsDir() {
		// Single pk3 file
		if strings.HasSuffix(strings.ToLower(quake3Dir), ".pk3") {
			return []string{quake3Dir}
		}
		return files
	}

	// Check if this directory has baseq3/missionpack structure
	hasStructure := false
	for _, subdir := range []string{"baseq3", "missionpack"} {
		if _, err := os.Stat(filepath.Join(quake3Dir, subdir)); err == nil {
			hasStructure = true
			break
		}
	}

	if hasStructure {
		// Process baseq3 and missionpack in order
		for _, subdir := range []string{"baseq3", "missionpack"} {
			dir := filepath.Join(quake3Dir, subdir)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				continue
			}
			files = append(files, collectPk3FilesFromDir(dir)...)
		}
	} else {
		// No standard structure, scan the directory directly
		files = collectPk3FilesFromDir(quake3Dir)
	}

	return files
}

// collectPk3FilesFromDir recursively collects pk3 files from a directory
// Returns them in Quake 3 load order: pak0-9 first, then others alphabetically
func collectPk3FilesFromDir(dir string) []string {
	var pakFiles []string   // pak[0-9].pk3 at root level
	var otherFiles []string // other pk3s

	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip directories we can't read
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".pk3") {
			return nil
		}

		name := d.Name()
		lowerName := strings.ToLower(name)

		// Only treat pak[0-9].pk3 at the root level specially
		isRootLevel := filepath.Dir(path) == dir
		if isRootLevel && strings.HasPrefix(lowerName, "pak") && len(lowerName) == 8 {
			numChar := lowerName[3]
			if numChar >= '0' && numChar <= '9' {
				pakFiles = append(pakFiles, path)
				return nil
			}
		}
		otherFiles = append(otherFiles, path)
		return nil
	})

	// Sort pak files numerically (pak0, pak1, ..., pak9)
	sort.Slice(pakFiles, func(i, j int) bool {
		return pakFiles[i] < pakFiles[j]
	})

	// Sort other files alphabetically
	sort.Strings(otherFiles)

	// Pak files first, then other files
	return append(pakFiles, otherFiles...)
}

// extractTgaToPng extracts a TGA file from a zip, scales to targetSize, and saves as PNG
func extractTgaToPng(f *zip.File, outputPath string, targetSize int) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	img, err := tga.Decode(rc)
	if err != nil {
		return fmt.Errorf("decode TGA: %w", err)
	}

	// Scale to target size using Catmull-Rom (bicubic) interpolation
	// CatmullRom produces sharper results than bilinear, better for pixel art
	bounds := img.Bounds()
	if bounds.Dx() != targetSize || bounds.Dy() != targetSize {
		dst := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		img = dst
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	return png.Encode(out, img)
}
